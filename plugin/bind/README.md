# bind

## Name

*bind* - overrides the host to which the server should bind.

## Description

Normally, the listener binds to the wildcard host. However, you may want the listener to bind to
another IP instead.      

If several addresses are provided, a listener will be open on each of the IP provided.

Each address has to be an IP of one of the interfaces of the host.

## Syntax

~~~ txt
bind ADDRESS [ADDRESS] ...
~~~

**ADDRESS** is an IP address to bind to.
When several addresses are provides a listener will be opened on each of the addresses

## Examples

To make your socket accessible only to that machine, bind to IP 127.0.0.1 (localhost):

~~~
. {
    bind 127.0.0.1
}
~~~

To allow processing DNS requests only local host on both Ipv4 and Ipv6 stacks, use the syntax:

~~~
. {
    bind 127.0.0.1 ::1
}
~~~
