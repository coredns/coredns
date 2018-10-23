# mdns

## Name

*mdns* - runs multicast DNS (mDNS) service for a domain.

## Description

TODO

## Syntax

~~~
mdns [ADDRESS]
~~~

If not specified, ADDRESS defaults to 0.0.0.0:5353.

## Examples

Enable mDNS service:

~~~
. {
    mdns
}
~~~

Listen on an alternate address and port, e.g. `10.20.30.40:5353`:

~~~ txt
. {
    mdns 10.20.30.40:5353
}
~~~

Listen on an all addresses on port 5353:

~~~ txt
. {
    mdns :5353
}
~~~

## Also See

See [Multicast DNS](https://en.wikipedia.org/wiki/Multicast_DNS).
