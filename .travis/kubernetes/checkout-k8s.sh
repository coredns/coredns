#!/bin/bash

[[ $DEBUG ]] && set -x

set -eof pipefail

# This is required to avoid using the unstable version (master branch)
# https://github.com/kubernetes/client-go#how-to-get-it
echo "Checking client-go"
CLIENT_GO=$GOPATH/src/k8s.io
mkdir -p $CLIENT_GO
cd $CLIENT_GO
git clone https://github.com/kubernetes/client-go
cd client-go
git checkout release-1.5
