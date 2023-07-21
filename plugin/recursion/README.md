# recursion

## Name

*recursion* - queries recursively for a resource.

## Description

*recursion* will repeat queries which return CNAME records, instead of the requested type, and try to hunt down the requested resource and return it to the client.
The recursive lookup is done transparently for the client.

Please be careful when exposing a DNS server to an untrusted network with recursion turned on.  The limitations built in can only go so far to prevent a server from hammering the internal DNS infrastructure.

Note:  DNS recursion can be expensive in both network load and CPU.  Memory maps are used to ensure that duplicate calls to the same resource are limited.  If a recursion plugin is followed by a foward out to a public internet, DNS recursion pointing to domains with layers upon layers of CNAME entries can ultimately cause DNS amplification of requests.  Likewise, recursion can speed up network resource requests as the DNS server can do all the additional queries needed to get to the intended record.

## Syntax

~~~
recursion {
    except IGNORED_NAMES...
    max_tries MAX
    max_depth MAX
    max_concurrent MAX
    timeout DURATION
}
~~~

* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from recursion. Requests that match none of these names will be passed through.

* `max_retries` **MAX** will limit the number of attempts to resolve a DNS entry.  This only applies when a domain has multiple CNAME entry option in a reply.  Each retry will follow a random path of CNAME resolutions until the desired record type is found.  (default 2)

* `max_depth` **MAX** will limit the depth of queries done.  A depth of 8 means a name can only be looked up 8 levels deep before giving up.  (default 8)

* `max_concurrent` **MAX** will limit the number of concurrent queries to MAX. Any new query that would raise the number of concurrent queries above the MAX will result in a REFUSED response. This response does not count as a health failure. When choosing a value for MAX, pick a number at least greater than the expected upstream query rate * latency of the upstream servers. As an upper bound for MAX, consider that each concurrent query will use about 2kb of memory.

## Examples

~~~ db1.txt
$ORIGIN example1.or.
@       3600 IN SOA sns.dns.icann.org. noc.dns.icann.org. 2017042745 7200 3600 1209600 3600
        3600 IN NS a.iana-servers.net.
        3600 IN NS b.iana-servers.net.

day     IN A 127.0.0.1
make    IN CNAME my.example2.or.
~~~

~~~ db2.txt
$ORIGIN example2.or.
@       3600 IN SOA sns.dns.icann.org. noc.dns.icann.org. 2017042745 7200 3600 1209600 3600
        3600 IN NS a.iana-servers.net.
        3600 IN NS b.iana-servers.net.

my      IN CNAME day.example1.or.
~~~

~~~ Corefile
.:10053 {
  recursion
  file db1.txt example1.or
  forward . 127.0.0.1:10054
}

.:10054 {
  file db2.txt example2.or
}
~~~

Command line dig example response:
```
$ dig @127.0.0.1 -p 10053 make.example1.or. +noall +answer

; <<>> DiG 9.11.4-P2-RedHat-9.11.4-26.P2.el7_9.13 <<>> @127.0.0.1 -p 10053 make.example1.or. +noall +answer
; (1 server found)
;; global options: +cmd
make.example1.or.       3600    IN      CNAME   my.example2.or.
my.example2.or.         3600    IN      CNAME   day.example1.or.
day.example1.or.        3600    IN      A       127.0.0.1
```
