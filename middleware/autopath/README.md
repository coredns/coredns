# autopath

*autopath* enables server side search path lookups. When enabled, it will identify search path
queries and perform the remaining lookups in the path on the client's behalf. 

A successful response will contain a question section with the original question, and an answer
section containing the record for the question that actually had an answer. This means that the
question and answer will not match. To avoid potential client confusion, a dynamically generated
CNAME entry is added to join the two. For example:

~~~ sh
  % host -v -t a google.com
  Trying "google.com.default.svc.cluster.local"
  ;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 50957
  ;; flags: qr rd ra; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 0

  ;; QUESTION SECTION:
  ;google.com.default.svc.cluster.local. IN A

  ;; ANSWER SECTION:
  google.com.default.svc.cluster.local. 175 IN CNAME google.com.
  google.com.		175	IN	A	216.58.194.206
~~~

## Syntax

~~~ txt
autopath [ZONE...] {
    ndots [NDOTS]
    response [RESPONSE]
    path NAME [NAME...]
    }
~~~

If **ZONE** is not defined use the one from the server block.

TODO: probably needs [ZONES] as well, and resolv-conf can be just a list of domain names.

* **NDOTS** (default: 0) This provides an adjustable threshold to prevent server side lookups from
  triggering. If the number of dots before the first search domain is less than this number, then
  the search path will not executed on the server side. When autopath is enabled with default
  settings, the search path is always conducted when the query is in the first search domain
  <pod-namespace>.svc.<zone>..
* **RESPONSE** (default: `NOERROR`) This option causes the kubernetes middleware to return the given
  response instead of `NXDOMAIN` when the all searches in the path produce no results. Valid values:
  `NXDOMAIN`, `SERVFAIL` or `NOERROR`. Setting this to `SERVFAIL` or `NOERROR` should prevent the client from
  fruitlessly continuing the client side searches in the path after the server already checked them.
* **RESOLV-CONF** (default: /etc/resolv.conf) If specified, the kubernetes middleware uses this file
  to get the host's search domains. The kubernetes middleware performs a lookup on these domains if
  the in-cluster search domains in the path fail to produce an answer. If not specified, the values
  will be read from the local resolv.conf file (i.e the resolv.conf file in the pod containing
  CoreDNS). In practice, this option should only need to be used if running CoreDNS outside of the
  cluster and the search path in /etc/resolv.conf does not match the cluster's "default"
  dns-policiy.

## Example

~~~ txt
autopath cluster.local. 0 NXDOMAIN /etc/resolv.conf
~~~
