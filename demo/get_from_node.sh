#!/bin/bash

CLUSTER_NAME=${1}
DOCUMENT_ID=${2}
POD_NAME=${3}

kubectl exec $POD_NAME --namespace=${CLUSTER_NAME} -- curl http://localhost:5984/${CLUSTER_NAME}/${DOCUMENT_ID}

