#!/bin/bash

CLUSTER_NAME=${1}
DOCUMENT_CONTENT=${2}
KUBERNETES_WORKER_NODE=172.17.4.201
SERVICE_NODE_PORT=`kubectl get svc couchdb --namespace=${CLUSTER_NAME} -o jsonpath=\{.spec.ports\[0\].nodePort\}`

curl -X POST \
  http://${KUBERNETES_WORKER_NODE}:${SERVICE_NODE_PORT}/${CLUSTER_NAME} \
  -H "Content-Type: application/json" \
  -d '{"hello":"world"}' | jq

