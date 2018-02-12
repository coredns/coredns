# bind

## Name

*bind* - overrides the host to which the server should bind.

## Description

Normally, the listener binds to the wildcard host. However, you may want the listener to bind to
another IP instead. If several addresses are provided, the listener will be duplicated such as each address is listened. 
This directive accepts several addresses, no ports.

## Syntax

~~~ txt
bind ADDRESS [ADDRESS] ...
~~~

**ADDRESS** is the IP address or list of IP addresses to bind to.

## Examples

To make your socket accessible only to that machine, bind to IP 127.0.0.1 (localhost):

~~~
. {
    bind 127.0.0.1
}
~~~

To duplicate the server and open on a second socket for the Ipv6 localhost counterpart:

~~~
. {
    bind 127.0.0.1 ::1
}
~~~

