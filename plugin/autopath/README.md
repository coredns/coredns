# autopath

## Name

*autopath* - allows for server-side search path completion.

## Description

If the *autopath* plugin sees a query that matches the first element of the configured search path, it will
follow the chain of search path elements and return the first reply that is not NXDOMAIN. On any
failures, the original reply is returned. Because *autopath* returns a reply for a name that wasn't
the original question, it will add a CNAME that points from the original name (with the search path
element in it) to the name of this answer.

**Note**: There are several known issues, see the "Bugs" section below.

## Syntax

~~~
autopath [ZONE...] RESOLV-CONF
~~~

* **ZONES** zones *autopath* should be authoritative for.
* **RESOLV-CONF** points to a `resolv.conf` like file or uses a special syntax to point to another
  plugin. For instance `@kubernetes`, will call out to the kubernetes plugin (for each
  query) to retrieve the search list it should use.

If a plugin implements the `AutoPather` interface then it can be used by *autopath*.

## Metrics

If monitoring is enabled (via the *prometheus* plugin) then the following metric is exported:

* `coredns_autopath_success_total{server}` - counter of successfully autopath-ed queries.

The `server` label is explained in the *metrics* plugin documentation.

## Examples

~~~
autopath my-resolv.conf
~~~

Use `my-resolv.conf` as the file to get the search path from. This file only needs to have one line:
`search domain1 domain2 ...`

~~~
autopath @kubernetes
~~~

Use the search path dynamically retrieved from the *kubernetes* plugin.

## Bugs

In Kubernetes, *autopath* can derive the wrong namespace of a client Pod (and therefore wrong search
path) in the following case. To properly build the search path of a client *autopath* needs to know
the namespace of the a Pod making a DNS request. To do this, it relies on the *kubernetes* plugin's
Pod cache to resolve the client's IP address to a Pod. The Pod cache is maintained by an API watch
on Pods. When Pod IP assignments change, the Kubernetes API notifies CoreDNS via the API watch.
However, that notification is not instantaneous. In the case that a Pod is deleted, and it's IP is
immediately provisioned to a Pod in another namespace, and that new Pod make a DNS lookup *before*
the API watch can notify CoreDNS of the change, *autopath* will resolve the IP to the previous Pod's
namespace.

In Kubernetes, *autopath* is not compatible with Pods running from Windows nodes.

If the server side search ultimately results in a negative answer (e.g. `NXDOMAIN`), then the client
will fruitlessly search all paths manually, thus negating the *autopath* optimization.
