#!/bin/bash

KUBECTL='./kubectl'

wait_until_k8s_ready() {
	# Wait until kubernetes is up and fully responsive
	while :
	do
   	 ${KUBECTL} get nodes 2>/dev/null | grep -q '127.0.0.1'
		if [ "${?}" = "0" ]; then
			break
		else
			echo "sleeping for 5 seconds"
			sleep 5
		fi
	done
	echo "kubernetes nodes:"
	$KUBECTL get nodes
}

wait_until_k8s_ready


echo "all cool"


