package couchdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uber-go/zap"

	"k8s.io/kubernetes/pkg/api"
	unversioned_api "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"k8s.io/kubernetes/pkg/kubectl/cmd"
	"k8s.io/kubernetes/pkg/util/intstr"
)

type Cluster struct {
	client       *unversioned.Client
	config       *restclient.Config
	logger       zap.Logger
	Namespace    string
	Heritage     string
	Name         string
	Replicas     int32
	ImageVersion string
	DatabaseName string
}

type ReplicationConfig struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Continuous bool   `json:"continuous"`
}

func newCluster(client *unversioned.Client, config *restclient.Config, namespace string, databaseName string) *Cluster {
	return &Cluster{
		client,
		config,
		zap.NewJSON(),
		namespace,
		"k8sdb",
		"couchdb",
		3,
		"couchdb:1.6.1",
		databaseName,
	}
}

func (c *Cluster) Create() error {
	c.LogInfo("Creating cluster")
	var err error = nil

	_, err = c.client.Namespaces().Create(c.namespaceStruct())
	c.LogInfo("Creating namespace")
	if err != nil {
		return err
	}

	_, err = c.client.Services(c.Namespace).Create(c.serviceStruct())
	c.LogInfo("Creating service")
	if err != nil {
		return err
	}

	_, err = c.client.Deployments(c.Namespace).Create(c.deploymentStruct())
	c.LogInfo("Creating deployment")
	if err != nil {
		return err
	}

	c.LogInfo("Waiting for cluster participants to be running")
	err = c.waitForClusterToBeRunning()
	if err != nil {
		return err
	}

	c.LogInfo("Configuring replication")
	err = c.configureReplication()
	if err != nil {
		return err
	}

	c.LogInfo("Cluster setup done")

	return nil
}

func (c *Cluster) waitForClusterToBeRunning() error {
	var err error = nil
	running := false
	for {
		running, err = c.areClusterParticipantsRunning()
		if err != nil {
			return err
		}

		if running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (c *Cluster) areClusterParticipantsRunning() (bool, error) {
	pods, err := c.client.Pods(c.Namespace).List(api.ListOptions{})
	if err != nil {
		return false, err
	}

	if len(pods.Items) != int(c.Replicas) {
		c.LogDebug("Not all replicas created yet")
		return false, nil
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" {
			c.LogDebug("At least one pod not running yet")
			return false, nil
		}
	}

	return true, nil
}

func (c *Cluster) configureReplication() error {
	pods, err := c.client.Pods(c.Namespace).List(api.ListOptions{})
	if err != nil {
		return err
	}

	time.Sleep(5000 * time.Millisecond)

	// make sure database exists on all nodes
	for _, pod := range pods.Items {
		c.ensureDatabaseExists(pod)
	}

	time.Sleep(5000 * time.Millisecond)

	// configure replication in a full mesh
	for _, pod := range pods.Items {
		for _, otherPod := range pods.Items {
			if pod.Status.PodIP != otherPod.Status.PodIP {
				err = c.configureSingleReplication(pod, otherPod)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Cluster) ensureDatabaseExists(pod api.Pod) error {
	return c.podExec(pod, []string{"curl", "-X", "PUT", c.databaseUrl("127.0.0.1")})
}

func (c *Cluster) databaseUrl(ip string) string {
	return fmt.Sprintf("http://%s:5984/%s", ip, c.DatabaseName)
}

func (c *Cluster) configureSingleReplication(pod api.Pod, otherPod api.Pod) error {
	replicationConfig := &ReplicationConfig{
		Source:     c.DatabaseName,
		Target:     fmt.Sprintf("http://%s:5984/%s", otherPod.Status.PodIP, c.DatabaseName),
		Continuous: true,
	}
	replicationConfigJson, err := json.Marshal(replicationConfig)

	if err != nil {
		return err
	}

	command := []string{
		"curl",
		"-v",
		"-X",
		"POST",
		"http://127.0.0.1:5984/_replicate",
		"-d",
		string(replicationConfigJson),
		"-H",
		"Content-Type: application/json",
	}

	return c.podExec(pod, command)
}

func (c *Cluster) podExec(pod api.Pod, command []string) error {
	podName := pod.ObjectMeta.Name
	podNamespace := pod.ObjectMeta.Namespace
	containerName := pod.Spec.Containers[0].Name

	options := &cmd.ExecOptions{
		In:            nil,
		Out:           new(bytes.Buffer),
		Err:           new(bytes.Buffer),
		PodName:       podName,
		ContainerName: containerName,
		Stdin:         false,
		TTY:           false,
		Command:       command,
		Namespace:     podNamespace,

		Executor: &cmd.DefaultRemoteExecutor{},
		Client:   c.client,
		Config:   c.config,
	}

	err := options.Validate()
	if err != nil {
		return err
	}

	return options.Run()
}

func (c *Cluster) Delete() error {
	c.LogInfo("Deleting cluster")
	return c.client.Namespaces().Delete(c.Namespace)
}

func (c *Cluster) namespaceStruct() *api.Namespace {
	return &api.Namespace{
		ObjectMeta: api.ObjectMeta{
			Name: c.Namespace,
		},
	}
}

func (c *Cluster) serviceStruct() *api.Service {
	return &api.Service{
		ObjectMeta: api.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
			Labels:    map[string]string{"name": c.Name, "heritage": c.Heritage},
		},
		Spec: api.ServiceSpec{
			Selector: map[string]string{"name": c.Name},
			Type:     "LoadBalancer",
			Ports:    []api.ServicePort{api.ServicePort{Port: 5984, TargetPort: intstr.IntOrString{IntVal: 5984}}},
		},
	}
}

func (c *Cluster) deploymentStruct() *extensions.Deployment {
	return &extensions.Deployment{
		ObjectMeta: api.ObjectMeta{
			Name:      c.Name,
			Namespace: c.Namespace,
			Labels:    map[string]string{"name": c.Name, "heritage": c.Heritage},
		},
		Spec: extensions.DeploymentSpec{
			Replicas: c.Replicas,
			Selector: &unversioned_api.LabelSelector{MatchLabels: map[string]string{"name": c.Name}},
			Template: api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Namespace: c.Namespace,
					Labels:    map[string]string{"name": c.Name, "heritage": c.Heritage},
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						api.Container{
							Name:  c.Name,
							Image: c.ImageVersion,
							LivenessProbe: &api.Probe{
								Handler: api.Handler{
									HTTPGet: &api.HTTPGetAction{
										Path: "/_stats",
										Port: intstr.IntOrString{IntVal: 5984},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (c *Cluster) LogInfo(message string) {
	c.logger.Info(message,
		zap.String("cluster", c.Namespace),
	)
}

func (c *Cluster) LogDebug(message string) {
	c.logger.Debug(message,
		zap.String("cluster", c.Namespace),
	)
}

func CreateCluster(namespace string) error {
	client, config, err := k8sClient()
	if err != nil {
		return err
	}

	err = newCluster(client, config, namespace, namespace).Create()
	if err != nil {
		return err
	}

	return nil
}

func DeleteCluster(namespace string) error {
	client, config, err := k8sClient()
	if err != nil {
		return err
	}

	err = newCluster(client, config, namespace, "").Delete()
	if err != nil {
		return err
	}

	return nil
}

func k8sClient() (*unversioned.Client, *restclient.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here
	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, err
	}

	client, err := unversioned.New(config)
	if err != nil {
		return nil, nil, err
	}

	return client, config, nil
}
