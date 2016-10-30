#!/bin/bash

[[ $DEBUG ]] && set -x

set -eof pipefail

KUBECTL='docker exec hyperkube /hyperkube kubectl'

PWD=`pwd`
cd `readlink -e $(dirname ${0})`

create_namespaces() {
	for n in ${NAMESPACES};
	do
		echo "Creating namespace: ${n}"
		${KUBECTL} create namespace ${n} || true
	done

	echo "kubernetes namespaces:"
	${KUBECTL} get namespaces
}

# run_and_expose_service <servicename> <namespace> <image> <port>
run_and_expose_service() {
	if [ "${#}" != "4" ]; then
		return -1
	fi

	service="${1}"
	namespace="${2}"
	image="${3}"
	port="${4}"

	echo "   starting service '${service}' in namespace '${namespace}'"
	${KUBECTL} run ${service} --namespace=${namespace} --image=${image}
	${KUBECTL} expose deployment ${service} --namespace=${namespace} --port=${port}
}

echo "Starting sample kubernetes services..."

NAMESPACES="demo poddemo test"
create_namespaces

echo ""
echo "Starting services:"

run_and_expose_service mynginx demo nginx 80
run_and_expose_service webserver demo nginx 80
run_and_expose_service mynginx test nginx 80
run_and_expose_service webserver test nginx 80
run_and_expose_service nginx-poddemo poddemo nginx 80

echo ""
echo "Services exposed:"
${KUBECTL} get services --all-namespaces

echo ""
echo "Deployments exposed:"
${KUBECTL} get deployments --all-namespaces

echo ""
echo "Pods running:"
${KUBECTL} get pods --all-namespaces

cd ${PWD}
