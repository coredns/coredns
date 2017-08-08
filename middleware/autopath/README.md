# autopath

The *autopath* middleware allows CoreDNS to perform server side search path completion.
If it sees a query that matches the first element of the configured search path, *autopath* will
follow the chain of search path elements and returns the first reply that is not NXDOMAIN.
On any failures the original reply is returned.

Because *autopath* returns a reply for a name that wasn't the original question it will add a CNAME
that points from the original name (with the search path element in it) to the name of this answer.

## Syntax

~~~
autopath [RESOLV-CONF]
~~~

* **RESOLV-CONF** points to the resolv.conf, a special syntax can be used to point to another
    middleware. For instance `@kubernetes`, will call out to the kubernetes middleware (for each
    query) to retrieve the search list it should use.

## Examples

~~~
autopath
~~~

  **NDOTS** (default: `0`) This provides an adjustable threshold to prevent server side lookups from triggering. If the number of dots before the first search domain is less than this number, then the search path will not executed on the server side.  When autopath is enabled with default settings, the search path is always conducted when the query is in the first search domain `<pod-namespace>.svc.<zone>.`.

  **RESPONSE** (default: `NOERROR`) This option causes the kubernetes middleware to return the given response instead of NXDOMAIN when the all searches in the path produce no results. Valid values: `NXDOMAIN`, `SERVFAIL` or `NOERROR`. Setting this to `SERVFAIL` or `NOERROR` should prevent the client from fruitlessly continuing the client side searches in the path after the server already checked them.

  **RESOLV-CONF** (default: `/etc/resolv.conf`) If specified, the kubernetes middleware uses this file to get the host's search domains. The kubernetes middleware performs a lookup on these domains if the in-cluster search domains in the path fail to produce an answer. If not specified, the values will be read from the local resolv.conf file (i.e the resolv.conf file in the pod containing CoreDNS).  In practice, this option should only need to be used if running CoreDNS outside of the cluster and the search path in /etc/resolv.conf does not match the cluster's "default" dns-policiy.

  Enabling autopath requires more memory, since it needs to maintain a watch on all pods. If autopath and `pods verified` mode are both enabled, they will share the same watch. Enabling both options should have an equivalent memory impact of just one.

  Example:

  ```
	kubernetes cluster.local. {
		autopath 0 NXDOMAIN /etc/resolv.conf
	}
  ```
