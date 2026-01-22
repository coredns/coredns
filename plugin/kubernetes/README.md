# kubernetes

## Name

*kubernetes* - enables reading zone data from a Kubernetes cluster.

## Description

This plugin implements the [Kubernetes DNS-Based Service Discovery
Specification](https://github.com/kubernetes/dns/blob/master/docs/specification.md).

CoreDNS running the kubernetes plugin can be used as a replacement for kube-dns in a kubernetes
cluster.  See the [deployment](https://github.com/coredns/deployment) repository for details on [how
to deploy CoreDNS in Kubernetes](https://github.com/coredns/deployment/tree/master/kubernetes).

[stubDomains and upstreamNameservers](https://kubernetes.io/blog/2017/04/configuring-private-dns-zones-upstream-nameservers-kubernetes/)
are implemented via the *forward* plugin. See the examples below.

This plugin can only be used once per Server Block.

## Syntax

~~~
kubernetes [ZONES...]
~~~

With only the plugin specified, the *kubernetes* plugin will default to the zone specified in
the server's block. It will handle all queries in that zone and connect to Kubernetes in-cluster. It
will not provide PTR records for services or A records for pods. If **ZONES** is used it specifies
all the zones the plugin should be authoritative for.

```
kubernetes [ZONES...] {
    endpoint URL
    tls CERT KEY CACERT
    kubeconfig KUBECONFIG [CONTEXT]
    apiserver_qps QPS
    apiserver_burst BURST
    apiserver_max_inflight MAX
    namespaces NAMESPACE...
    labels EXPRESSION
    pods POD-MODE
    endpoint_pod_names
    ttl TTL
    noendpoints
    fallthrough [ZONES...]
    ignore empty_service
    multicluster [ZONES...]
    startup_timeout DURATION
}
```

* `endpoint` specifies the **URL** for a remote k8s API endpoint.
   If omitted, it will connect to k8s in-cluster using the cluster service account. Needs `tls` for clusters with authentication.
   This option is ignored if `kubeconfig` is set.
* `tls` **CERT** **KEY** **CACERT** are the TLS cert, key and the CA cert file names for remote k8s connection.
   This option is ignored if connecting in-cluster (i.e. endpoint is not specified).
* `kubeconfig` **KUBECONFIG [CONTEXT]** authenticates the connection to a remote k8s cluster using a kubeconfig file.
   **[CONTEXT]** is optional, if not set, then the current context specified in kubeconfig will be used.
   It supports TLS, username and password, or token-based authentication.
   This option is ignored if omitted. The cluster address in the `kubeconfig` is given preference.
* `apiserver_qps` **QPS** sets the maximum queries per second (QPS) rate limit for requests.
   This allows you to control the rate at which the plugin sends requests to the API server to prevent overwhelming it.
* `apiserver_burst` **BURST** sets the maximum burst size for requests.
   This allows temporary spikes in request rate up to this value, even if it exceeds the QPS limit.
* `apiserver_max_inflight` **MAX** sets the maximum number of concurrent in-flight requests.
   This caps the total number of simultaneous requests the plugin can make to the API server.
* `namespaces` **NAMESPACE [NAMESPACE...]** only exposes the k8s namespaces listed.
   If this option is omitted all namespaces are exposed
* `namespace_labels` **EXPRESSION** only expose the records for Kubernetes namespaces that match this label selector.
   The label selector syntax is described in the
   [Kubernetes User Guide - Labels](https://kubernetes.io/docs/user-guide/labels/). An example that
   only exposes namespaces labeled as "istio-injection=enabled", would use:
   `labels istio-injection=enabled`.
* `labels` **EXPRESSION** only exposes the records for Kubernetes objects that match this label selector.
   The label selector syntax is described in the
   [Kubernetes User Guide - Labels](https://kubernetes.io/docs/user-guide/labels/). An example that
   only exposes objects labeled as "application=nginx" in the "staging" or "qa" environments, would
   use: `labels environment in (staging, qa),application=nginx`.
* `pods` **POD-MODE** sets the mode for handling IP-based pod A records, e.g.
   `1-2-3-4.ns.pod.cluster.local. in A 1.2.3.4`.
   This option is provided to facilitate use of SSL certs when connecting directly to pods. Valid
   values for **POD-MODE**:

   * `disabled`: Default. Do not process pod requests, always returning `NXDOMAIN`
   * `insecure`: Always return an A record with IP from request (without checking k8s).  This option
     is vulnerable to abuse if used maliciously in conjunction with wildcard SSL certs.  This
     option is provided for backward compatibility with kube-dns.
   * `verified`: Return an A record if there exists a pod in same namespace with matching IP.  This
     option requires substantially more memory than in insecure mode, since it will maintain a watch
     on all pods.

* `endpoint_pod_names` uses the pod name of the pod targeted by the endpoint as
   the endpoint name in A records, e.g.,
   `endpoint-name.my-service.namespace.svc.cluster.local. in A 1.2.3.4`
   By default, the endpoint-name name selection is as follows: Use the hostname
   of the endpoint, or if hostname is not set, use the dashed form of the endpoint
   IP address (e.g., `1-2-3-4.my-service.namespace.svc.cluster.local.`)
   If this directive is included, then name selection for endpoints changes as
   follows: Use the hostname of the endpoint, or if hostname is not set, use the
   pod name of the pod targeted by the endpoint. If there is no pod targeted by
   the endpoint or pod name is longer than 63, use the dashed IP address form.
* `ttl` allows you to set a custom TTL for responses. The default is 5 seconds.  The minimum TTL allowed is
  0 seconds, and the maximum is capped at 3600 seconds. Setting TTL to 0 will prevent records from being cached.
* `noendpoints` will turn off the serving of endpoint records by disabling the watch on endpoints.
  All endpoint queries and headless service queries will result in an NXDOMAIN.
* `fallthrough` **[ZONES...]** If a query for a record in the zones for which the plugin is authoritative
  results in NXDOMAIN, normally that is what the response will be. However, if you specify this option,
  the query will instead be passed on down the plugin chain, which can include another plugin to handle
  the query. If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin
  is authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then only
  queries for those zones will be subject to fallthrough.
* `ignore empty_service` returns NXDOMAIN for services without any ready endpoint addresses (e.g., ready pods).
  This allows the querying pod to continue searching for the service in the search path.
  The search path could, for example, include another Kubernetes cluster.
* `multicluster` defines the multicluster zones as defined by Multi-Cluster
  Services API (MCS-API). Specifying this option is generally paired with the
  installation of an MCS-API implementation and the ServiceImport and ServiceExport
  CRDs. The plugin MUST be authoritative for the zones listed here.
* `startup_timeout` specifies the **DURATION** value that limits the time to wait for informer cache synced
  when the kubernetes plugin starts. If not specified, the default timeout will be 5s.

Enabling zone transfer is done by using the *transfer* plugin.

## Startup

When CoreDNS starts with the *kubernetes* plugin enabled, it will delay serving DNS for up to 5 seconds
until it can connect to the Kubernetes API and synchronize all object watches.  If this cannot happen within
5 seconds, then CoreDNS will start serving DNS while the *kubernetes* plugin continues to try to connect
and synchronize all object watches.  CoreDNS will answer SERVFAIL to any request made for a Kubernetes record
that has not yet been synchronized. You can also determine how long to wait by specifying `startup_timeout`.

## Monitoring Kubernetes Endpoints

The *kubernetes* plugin watches Endpoints via the `discovery.EndpointSlices` API.

## Ready

This plugin reports readiness to the ready plugin. This will happen after it has synced to the
Kubernetes API.

## PTR Records

This plugin creates PTR records for every Pod selected by a Service. If a given Pod is selected by more than
one Service a separate PTR record will exist for each Service selecting it.

## Examples

Handle all queries in the `cluster.local` zone. Connect to Kubernetes in-cluster. Also handle all
`in-addr.arpa` `PTR` requests for `10.0.0.0/17` . Verify the existence of pods when answering pod
requests.

~~~ txt
10.0.0.0/17 cluster.local {
    kubernetes {
        pods verified
    }
}
~~~

Or you can selectively expose some namespaces:

~~~ txt
kubernetes cluster.local {
    namespaces test staging
}
~~~

Connect to Kubernetes with CoreDNS running outside the cluster:

~~~ txt
kubernetes cluster.local {
    endpoint https://k8s-endpoint:8443
    tls cert key cacert
}
~~~

Configure multicluster

~~~ txt
kubernetes cluster.local clusterset.local {
    multicluster clusterset.local
}
~~~

## stubDomains and upstreamNameservers

Here we use the *forward* plugin to implement a stubDomain that forwards `example.local` to the nameserver `10.100.0.10:53`.
Also configured is an upstreamNameserver `8.8.8.8:53` that will be used for resolving names that do not fall in `cluster.local`
or `example.local`.

~~~ txt
cluster.local:53 {
    kubernetes cluster.local
}
example.local {
    forward . 10.100.0.10:53
}

. {
    forward . 8.8.8.8:53
}
~~~

The configuration above represents the following Kube-DNS stubDomains and upstreamNameservers configuration.

~~~ txt
stubDomains: |
   {“example.local”: [“10.100.0.10:53”]}
upstreamNameservers: |
   [“8.8.8.8:53”]
~~~

## AutoPath

The *kubernetes* plugin can be used in conjunction with the *autopath* plugin.  Using this
feature enables server-side domain search path completion in Kubernetes clusters.  Note: `pods` must
be set to `verified` for this to function properly. Furthermore, the remote IP address in the DNS
packet received by CoreDNS must be the IP address of the Pod that sent the request.

    cluster.local {
        autopath @kubernetes
        kubernetes {
            pods verified
        }
    }

## Metadata

The kubernetes plugin will publish the following metadata, if the *metadata*
plugin is also enabled:

 * `kubernetes/endpoint`: the endpoint name in the query
 * `kubernetes/kind`: the resource kind (pod or svc) in the query
 * `kubernetes/namespace`: the namespace in the query
 * `kubernetes/port-name`: the port name in an SRV query
 * `kubernetes/protocol`: the protocol in an SRV query
 * `kubernetes/service`: the service name in the query
 * `kubernetes/client-namespace`: the client pod's namespace (see requirements below)
 * `kubernetes/client-pod-name`: the client pod's name (see requirements below)
 * `kubernetes/client-label/<label key>`: a label on the client pod (see requirements below)

The `kubernetes/client-namespace`, `kubernetes/client-pod-name`, and `kubernetes/client-label/<label key>`
metadata work by reconciling the client IP address in the DNS request packet to a known pod IP address.
Therefore the following is required:
 * `pods verified` mode must be enabled
 * the remote IP address in the DNS packet received by CoreDNS must be the IP address
   of the Pod that sent the request.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metrics are exported:

* `coredns_kubernetes_dns_programming_duration_seconds{service_kind}` - Exports the
  [DNS programming latency SLI](https://github.com/kubernetes/community/blob/master/sig-scalability/slos/dns_programming_latency.md).
  The metrics has the `service_kind` label that identifies the kind of the
  [kubernetes service](https://kubernetes.io/docs/concepts/services-networking/service).
  It may take one of the three values:
    * `cluster_ip`
    * `headless_with_selector`
    * `headless_without_selector`

The following are client level metrics to monitor apiserver request latency & status codes. `verb` identifies the apiserver [request type](https://kubernetes.io/docs/reference/using-api/api-concepts/#single-resource-api) and `host` denotes the apiserver endpoint.
* `coredns_kubernetes_rest_client_request_duration_seconds{verb, host}` - captures apiserver request latency perceived by client grouped by `verb` and `host`.
* `coredns_kubernetes_rest_client_rate_limiter_duration_seconds{verb, host}` - captures apiserver request latency contributed by client side rate limiter grouped by `verb` & `host`.
* `coredns_kubernetes_rest_client_requests_total{method, code, host}` - captures total apiserver requests grouped by `method`, `status_code` & `host`.

## Bugs

The duration metric only supports the "headless\_with\_selector" service currently.

## See Also

See the *autopath* plugin to enable search path optimizations. And use the *transfer* plugin to
enable outgoing zone transfers.
