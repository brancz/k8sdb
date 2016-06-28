package couchdb

import (
	"fmt"
	"time"

	"k8s.io/kubernetes/pkg/api"
	unversioned_api "k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/apis/extensions"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	"k8s.io/kubernetes/pkg/client/unversioned/remotecommand"
	remotecommandserver "k8s.io/kubernetes/pkg/kubelet/server/remotecommand"
	"k8s.io/kubernetes/pkg/util/intstr"
)

type Cluster struct {
	client       *unversioned.Client
	config       *restclient.Config
	Namespace    string
	Heritage     string
	Name         string
	Replicas     int32
	ImageVersion string
	DatabaseName string
}

func newCluster(client *unversioned.Client, config *restclient.Config, namespace string) *Cluster {
	return &Cluster{client, config, namespace, "k8sdb", "couchdb", 3, "couchdb:1.6.1", "test123"}
}

func (c *Cluster) Create() error {
	namespace, err := c.client.Namespaces().Create(c.namespaceStruct())
	fmt.Println("Creating namespace")
	fmt.Println(namespace)
	fmt.Println(err)
	if err != nil {
		return err
	}

	service, err := c.client.Services(c.Namespace).Create(c.serviceStruct())
	fmt.Println("Creating service")
	fmt.Println(service)
	fmt.Println(err)
	if err != nil {
		return err
	}

	deployment, err := c.client.Deployments(c.Namespace).Create(c.deploymentStruct())
	fmt.Println("Creating deployment")
	fmt.Println(deployment)
	fmt.Println(err)
	if err != nil {
		return err
	}

	err = c.waitForClusterToBeRunning()
	if err != nil {
		return err
	}

	err = c.configureReplication()
	if err != nil {
		return err
	}

	fmt.Println("Cluster setup done")

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
		fmt.Println("Not all replicas created yet")
		return false, nil
	}

	for _, pod := range pods.Items {
		fmt.Println(pod.Status.Phase)
		if pod.Status.Phase != "Running" {
			fmt.Println("At least one pod not running yet")
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

	// configure replication in a full mesh
	for _, pod := range pods.Items {
		c.ensureDatabaseExists(pod)
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

func (c *Cluster) configureSingleReplication(pod api.Pod, otherPod api.Pod) error {
	return nil
	//c.podExec(pod, []string{"curl", "-X", "POST", fmt.Sprintf("%s/_replicate", c.databaseUrl("127.0.0.1")), "-d", fmt.Sprintf("{\"source\":\"%s\",\"target\":\"http://%s:5984/%s\",\"continuous\":\"true\"}", c.DatabaseName, otherPod.Status.PodIP, c.DatabaseName), "-H", "\"Content-Type: application/json\""})
}

func (c *Cluster) databaseUrl(ip string) string {
	return fmt.Sprintf("http://%s:5984/%s", ip, c.DatabaseName)
}

func (c *Cluster) podExec(pod api.Pod, command []string) error {
	podName := pod.ObjectMeta.Name
	podNamespace := pod.ObjectMeta.Namespace
	containerName := pod.Spec.Containers[0].Name

	req := c.client.RESTClient.Post().
		Resource("pods").
		Name(podName).
		Namespace(podNamespace).
		SubResource("exec").
		Param("container", containerName)
	req.VersionedParams(&api.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdin:     false,
		Stdout:    false,
		Stderr:    false,
		TTY:       false,
	}, api.ParameterCodec)

	fmt.Println(req.URL())
	exec, err := remotecommand.NewExecutor(c.config, "POST", req.URL())

	if err != nil {
		return err
	}

	return exec.Stream(remotecommandserver.SupportedStreamingProtocols, nil, nil, nil, false)
}

func (c *Cluster) Delete() error {
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
					Containers: []api.Container{api.Container{Name: c.Name, Image: c.ImageVersion}},
				},
			},
		},
	}
}

func CreateCluster(namespace string) error {
	fmt.Println(fmt.Sprintf("creating cluster: %s", namespace))
	client, config, err := k8sClient()
	if err != nil {
		return err
	}

	err = newCluster(client, config, namespace).Create()
	if err != nil {
		return err
	}

	return nil
}

func DeleteCluster(namespace string) error {
	fmt.Println(fmt.Sprintf("deleteing cluster: %s", namespace))
	client, config, err := k8sClient()
	if err != nil {
		return err
	}

	err = newCluster(client, config, namespace).Delete()
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
