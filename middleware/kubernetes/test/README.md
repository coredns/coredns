## Test scripts to automate kubernetes startup

Requirements:
	docker
	curl

The scripts in this directory startup kubernetes with docker as the container runtime.
After starting kubernetes, a couple of kubernetes services are started to allow automatic
testing of CoreDNS with kubernetes.

To use, run the scripts as:

~~~
$ ./00_run_k8s.sh && ./10_setup_kubectl.sh && ./20_setup_k8s_services.sh
~~~

Kubernetes and all running containers can be uncerimoniously stopped by
running the `kill_all_containers.sh` script.

~~~
$ ./kill_all_containers.sh
~~~
