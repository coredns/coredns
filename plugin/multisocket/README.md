# multisocket

## Name

*multisocket* - allows to start multiple servers that will listen on one port.

## Description

With *multisocket*, you can define the number of servers that will listen on the same port. The SO_REUSEPORT socket 
option allows to open multiple listening sockets at the same address and port. In this case, kernel distributes incoming 
connections between sockets.

Enabling this option allows to start multiple servers, which increases the throughput of CoreDNS in environments with a 
large number of CPU cores.

## Syntax

~~~
multisocket [NUM_SOCKETS]
~~~

* **NUM_SOCKETS** - the number of servers that will listen on one port. Default value is equal to GOMAXPROCS. Maximum value is 1024.

## Examples

Start 5 TCP/UDP servers on the same port.

~~~ corefile
. {
	multisocket 5
	forward . /etc/resolv.conf
}
~~~

Do not define `NUM_SOCKETS`, in this case it will take a value equal to GOMAXPROCS.

~~~ corefile
. {
	multisocket
	forward . /etc/resolv.conf
}
~~~

## Recommendations

The tests of the `multisocket` plugin, which were conducted for `NUM_SOCKETS` from 1 to 10, did not reveal any side 
effects or performance degradation.

This means that the `multisocket` plugin can be used with a default value that is equal to GOMAXPROCS.

However, to achieve the best results, it is recommended to consider the specific environment and plugins used in 
CoreDNS. To determine the optimal configuration, it is advisable to conduct performance tests with different 
`NUM_SOCKETS`, measuring Queries Per Second (QPS) and system load.

If conducting such tests is difficult, follow these recommendations:
1. Determine the maximum CPU consumption of CoreDNS server without `multisocket` plugin. Estimate how much CPU CoreDNS
   actually consumes in specific environment under maximum load.
2. Align `NUM_SOCKETS` with the estimated CPU usage and CPU limits or system's available resources.
   Examples:
   - If CoreDNS consumes 4 CPUs and 8 CPUs are available, set `NUM_SOCKETS` to 2.
   - If CoreDNS consumes 8 CPUs and 64 CPUs are available, set `NUM_SOCKETS` to 8.

## Limitations

The `multisocket` value used for a given listen address is taken from the first server block that binds to that address
in the Corefile. Subsequent server blocks using the same address will not change it. Different addresses may use
different values.

The SO_REUSEPORT socket option is not available for some operating systems. It is available since Linux Kernel 3.9 and 
not available for Windows at all.

Using this plugin with a system that does not support SO_REUSEPORT will cause an `address already in use` error.
