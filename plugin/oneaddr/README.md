# oneaddr

## Name

*oneaddr* - filters response and retains only one (first) address

## Description

*oneaddr* will remove all addresses from response except first one.
It is intended to be used with loadbalance plugin.

## Syntax

~~~
oneaddr
~~~

## Examples

Let's consider following configuration for example.org zone. We aim to enable
round-robin loadbalancing, but ensure fair distribution and reveal just one
worker server in the DNS response.

Zone file `db.example.org`:

~~~
$ORIGIN example.org.
@	3600 IN	SOA sns.dns.icann.org. noc.dns.icann.org. 2017042745 7200 3600 1209600 3600
	3600 IN NS a.iana-servers.net.
	3600 IN NS b.iana-servers.net.

www     IN A     127.0.0.1
www     IN A     127.0.0.2
www     IN A     127.0.0.3
www     IN A     127.0.0.4
www     IN A     127.0.0.5
www     IN A     127.0.0.6
www     IN A     127.0.0.7
www     IN A     127.0.0.8
www     IN A     127.0.0.9
www     IN A     127.0.0.10
~~~

CoreDNS configuration:

~~~ corefile
example.org {
	file db.example.org
	loadbalance
	oneaddr
}
~~~

With such configuration only one A-record will be present in the DNS response.
Since *loadbalance* module randomizes order and *oneaddr* picks first address,
IP address in the responses will vary and distribute load across worker servers.
