# corends-multicluster

## Name

*multicluster* - implementation of [Multicluster DNS](https://github.com/kubernetes/enhancements/pull/2577)

## Description

This plugin implements the [Kubernetes DNS-Based Multicluster Service Discovery
Specification](https://github.com/kubernetes/enhancements/pull/2577).

## Syntax

```
multicluster [ZONES...] {
    kubeconfig KUBECONFIG [CONTEXT]
    noendpoints
    fallthrough [ZONES...]
}
```

* `kubeconfig` **KUBECONFIG [CONTEXT]** authenticates the connection to a remote k8s cluster using a kubeconfig file. **[CONTEXT]** is optional, if not set, then the current context specified in kubeconfig will be used. It supports TLS, username and password, or token-based authentication. This option is ignored if connecting in-cluster (i.e., the endpoint is not specified).
* `noendpoints` will turn off the serving of endpoint records by disabling the watch on endpoints. All endpoint queries and headless service queries will result in an NXDOMAIN.
* `fallthrough` **[ZONES...]** If a query for a record in the zones for which the plugin is authoritative results in NXDOMAIN, normally that is what the response will be. However, if you specify this option, the query will instead be passed on down the plugin chain, which can include another plugin to handle the query. If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin is authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then only queries for those zones will be subject to fallthrough.

## Startup

When CoreDNS starts with the *multicluster* plugin enabled, it will delay serving DNS for up to 5 seconds until it can connect to the Kubernetes API and synchronize all object watches. If this cannot happen within 5 seconds, then CoreDNS will start serving DNS while the *multicluster* plugin continues to try to connect and synchronize all object watches.  CoreDNS will answer SERVFAIL to any request made for a Kubernetes record that has not yet been synchronized.

## Examples

Handle all queries in the `clusterset.local` zone. Connect to Kubernetes in-cluster.

```
.:53 {
    multicluster clusterset.local
}
```

## Installation

See CoreDNS documentation about [Compile Time Enabling or Disabling Plugins](https://coredns.io/2017/07/25/compile-time-enabling-or-disabling-plugins/).

### Recompile coredns

Add the plugin to  `plugins.cfg` file. The [ordering of plugins matters](https://coredns.io/2017/06/08/how-queries-are-processed-in-coredns/),
add it just below `kubernetes` plugin that has very similar functionality:

```
...
kubernetes:kubernetes
multicluster:github.com/coredns/coredns/plugin/multicluster
...
```

Follow the [coredns README](https://github.com/coredns/coredns#readme) file to build it.

### Modify cluster's corefile

To enable the plugin for `clusterset.local` zone, add `multicluster` configuration to the `corefile`. Resulting `corefile` may look like this:

```
.:53 {
    errors
    health
    multicluster clusterset.local
    kubernetes cluster.local in-addr.arpa ip6.arpa {
      pods insecure
      fallthrough in-addr.arpa ip6.arpa
    }
    prometheus :9153
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}
```
