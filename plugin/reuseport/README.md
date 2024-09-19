# reuseport

## Name

*reuseport* - allows to define the number of servers that will listen on one port.

## Description

With *reuseport*, you can define the number of servers that will listen on the same port. The SO_REUSEPORT socket option
allows to open multiple listening sockets at the same address and port. In this case, kernel distributes incoming 
connections between sockets.

Enabling this option allows to start multiple servers, which increases the throughput of CoreDNS in environments with a 
large number of CPU cores.

## Syntax

~~~
reuseport [NUM_SOCKS]
~~~

* **NUM_SOCKS** - the number of servers that will listen on one port.

## Examples

Start 5 TCP/UDP servers on port 53.

~~~ corefile
.:53 {
	reuseport 5
	forward . /etc/resolv.conf
}
~~~

## Limitations

The SO_REUSEPORT socket option is not available for some operating systems. It is available since Linux Kernel 3.9 and 
not available for Windows at all.

Using this plugin with a system that does not support SO_REUSEPORT will cause an `address already in use` error.
