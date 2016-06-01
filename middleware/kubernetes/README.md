Kubernetes Middleware
=====================

Overview
--------

In Kubernetes a service has a _name_ and is launched within a _namespace_.


Implementation Ideas
--------------------

The middleware is configured with a "zone" string. For example: "zone = coredns.local".

The Kubernetes service "myservice" running in "mynamespace" would map 
to: "myservice.mynamespace.coredns.local".

The middleware should publish an A record for that service and a service record.

Initial implementation just performs the above simple mapping. Subsequent 
revisions should allow different namespaces to be published under different zones.
For example:

    # Serve on port 1053
    .:1053 {
    # use kubernetes middleware for domain "coredns.local" for namespaces "staging" and "test"
        kubernetes coredns.local staging, test {
            # Use url for k8s API endpoint
            endpoint http://localhost:8080
        }
    # use kubernetes middleware for domain "prod.local" for namespace "prod
    kubernetes prod.local prod {
            # Use url for k8s API endpoint
            endpoint http://localhost:8080
        }
    }


### Internal IP or External IP?
Should the Corefile configuration allow control over whether the internal IP or external IP is exposed? Also control the precidence?

For example a service "myservice" running in namespace "mynamespace" with internal IP "10.0.0.100" and external IP "1.2.3.4".

This example could be published as:

| Corefile directive           | Result              |
|------------------------------|---------------------|
| iporder = internal           | 10.0.0.100          |
| iporder = external           | 1.2.3.4             |
| iporder = external, internal | 10.0.0.100, 1.2.3.4 |
| iporder = internal, external | 1.2.3.4, 10.0.0.100 |
| _no directive_               | 10.0.0.100, 1.2.3.4 |



### Wildcards

Publishing DNS records for singleton services isn't very interesting. Service
names are unique within a k8s namespace therefore multiple services will be
commonly run with a structured naming scheme.

For example, running multiple nginx services under the names:

| Service name |
|--------------|
| c1.nginx     |
| c2.nginx     |

or:

| Service name |
|--------------|
| nginx.c3     |
| nginx.c4     |

A DNS query with wildcard support for "nginx" in these examples should
return the IP addresses for all services with "nginx" in the service name.

TBD:
* How does this relate the the k8s load-balancer configuration?
* Do wildcards search across namespaces?


TODO
----
* Implement naive lookup against k8s API.
* Implement A-record queries using naive lookup.
* Implement namespace filtering to different zones.
* Implement IP selection and ordering (internal/external).
* Implement SRV-record queries using naive lookup.
* Flatten service and namespace names to valid DNS characters. (service names
  and namespace names in k8s may use uppercase and non-DNS characters. Implement
  flattening to lower case and mapping of non-DNS characters to DNS characters
  in a standard way.)
* Do we need to generate synthetic zone records for namespaces?
* Implement wildcard-based lookup.
* Improve lookup to reduce size of query result (namespace-based?, other ideas?)
* How to support label specification in Corefile to allow use of labels to indicate zone? (Is this even useful?)
* Test with CoreDNS caching. Is this enough, or do we need caching wrapped around the http API query?
