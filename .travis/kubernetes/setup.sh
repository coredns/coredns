#!/bin/bash
set -v

curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 && chmod +x minikube
curl -Lo kubectl  https://storage.googleapis.com/kubernetes-release/release/v1.7.0/bin/linux/amd64/kubectl && chmod +x kubectl
sudo mv ./minikube /usr/local/bin/
sudo mv ./kubectl /usr/local/bin/

export MINIKUBE_WANTUPDATENOTIFICATION=false
export MINIKUBE_WANTREPORTERRORPROMPT=false
export MINIKUBE_HOME=$HOME
export CHANGE_MINIKUBE_NONE_USER=true
mkdir $HOME/.kube || true
touch $HOME/.kube/config

export KUBECONFIG=$HOME/.kube/config
sudo -E minikube start --vm-driver=none

# Wait for kubernetes api service to be ready
for i in {1..150} # timeout for 5 minutes
do
   kubectl get po
   if [ $? -ne 1 ]; then
      break
  fi
  sleep 2
done

# Disable kube-dns in addon manager
sudo minikube addons disable kube-dns

# Manually delete kube-dns components
kubectl -n kube-system delete deployment kube-dns
kubectl -n kube-system delete service kube-dns

# Deploy test objects
kubectl create -f ./.travis/kubernetes/dns-test.yaml

# Start a local docker repository
docker run -d -p 5000:5000 --restart=always --name registry registry:2.6.2

# Build and push coredns docker image to local repository
make coredns SYSTEM="GOOS=linux"
docker build -t coredns .
docker tag coredns localhost:5000/coredns
docker push localhost:5000/coredns

# Deploy coredns in place of kube-dns
kubectl apply -f ./.travis/kubernetes/coredns.yaml

# Wait for coredns to be ready
for i in {1..150} # timeout for 5 minutes
do
  kubectl -n kube-system get pods | grep coredns
  kubectl -n kube-system get pods | grep coredns | grep Running && break
  sleep 2
done

# Wait for all test pods in test-1 to be ready (there are 5)
for i in {1..150} # timeout for 5 minutes
do
  kubectl -n test-1 get pods
  [ `kubectl -n test-1 get pods | grep Running | wc -l` == 5 ] && break
  sleep 2
done

