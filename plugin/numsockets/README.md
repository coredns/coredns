# numsockets

## Name

*numsockets* - allows to define the number of servers that will listen on one port.

## Description

With *numsockets*, you can define the number of servers that will listen on the same port. The SO_REUSEPORT socket 
option allows to open multiple listening sockets at the same address and port. In this case, kernel distributes incoming 
connections between sockets.

Enabling this option allows to start multiple servers, which increases the throughput of CoreDNS in environments with a 
large number of CPU cores.

## Syntax

~~~
numsockets NUM_SOCKETS
~~~

* **NUM_SOCKETS** - the number of servers that will listen on one port.

## Examples

Start 5 TCP/UDP servers on the same port.

~~~ corefile
. {
	numsockets 5
	forward . /etc/resolv.conf
}
~~~

## Recommendations

When choosing the optimal `numsockets` value, it is important to consider the specific environment and plugins used in 
CoreDNS. To determine the optimal value, it is advisable to conduct performance tests with different `numsockets`, 
measuring Queries Per Second (QPS) and system load.

If conducting such tests is difficult, follow these recommendations:
1. Determine the maximum CPU consumption of CoreDNS server without `numsockets` plugin. Estimate how much CPU CoreDNS
   actually consumes in specific environment under maximum load.
2. Align `numsockets` with the estimated CPU usage and CPU limits or system's available resources.
   Examples:
   - If CoreDNS consumes 4 CPUs and 8 CPUs are available, set `numsockets` to 2.
   - If CoreDNS consumes 8 CPUs and 64 CPUs are available, set `numsockets` to 8.

**Important:**
- Tests have shown that increasing `numsockets` above 8 does not improve performance. Therefore, it is advised to use 
  values greater than 8 only if performance tests justify such an increase.
- Reaching a CPU limit means that increasing `numsockets` will not be useful. With the same CPU limit, increasing 
  `numsockets` can decrease QPS. Use the plugin and set more `numsockets` only if absolutely necessary.

## Limitations

The SO_REUSEPORT socket option is not available for some operating systems. It is available since Linux Kernel 3.9 and 
not available for Windows at all.

Using this plugin with a system that does not support SO_REUSEPORT will cause an `address already in use` error.
