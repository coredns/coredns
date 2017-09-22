#!/bin/bash

KUBECTL="docker exec hyperkube /hyperkube kubectl"

# Start hyperkube container
docker run -d \
    --volume=/:/rootfs:ro \
    --volume=/sys:/sys:ro \
    --volume=/var/lib/docker/:/var/lib/docker:rw \
    --volume=/var/lib/kubelet/:/var/lib/kubelet:rw \
    --volume=/var/run:/var/run:rw \
    --volume=`pwd`/.travis:/travis \
    --net=host --pid=host --privileged \
    --name=hyperkube gcr.io/google_containers/hyperkube-amd64:$K8S_VERSION \
    /hyperkube \
    kubelet \
        --containerized \
        --hostname-override=127.0.0.1 \
        --api-servers=http://localhost:8080 \
        --config=/etc/kubernetes/manifests \
        --allow-privileged --v=2

# Wait until kubectl is ready
for i in {1..10}; do $KUBECTL version && break || sleep 5; done

# Set up kubectl config context
$KUBECTL config set-cluster test-doc --server=http://localhost:8080
$KUBECTL config set-context test-doc --cluster=test-doc
$KUBECTL config use-context test-doc

# Wait until k8s api is ready
for i in {1..30}; do $KUBECTL get nodes && break || sleep 5; done

# Create test objects
$KUBECTL create -f /travis/kubernetes/dns-test.yaml

