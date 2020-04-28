# k8s_node

## Name

*k8s_node* - resolves node hostname and node IPs from Kubernetes clusters.

## Description

This plugin resolves node external IP and internal IP address(es) of Kubernetes clusters. 
This plugin is only useful if the *kubernetes* plugin is also loaded.

The plugin resolve node IP addresses. It only handles queries for A and AAAA records;
all others result in NODATA responses.

## Syntax

~~~
k8s_node
~~~

If you want to change the apex domain or use a different TTL for the returned records you can use
this extended syntax.

~~~
k8s_node {
    ttl TTL
}
~~~

* `ttl` allows you to set a custom **TTL** for responses. The default is 5 (seconds).

## Examples

~~~
. {
   kubernetes cluster.local
   k8s_node
}
~~~

With the Corefile above, the following Service will get an `A` record for `vm10-0-10-80.ksc.com` with the IP address `10.0.10.80`.

~~~
status:
  addresses:
  - address: 10.0.10.80
    type: InternalIP
  - address: vm10-0-10-80.ksc.com
    type: Hostname
~~~

# Bugs

PTR queries for the reverse zone is not supported.