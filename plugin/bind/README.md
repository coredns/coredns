# bind

## Name

*bind* - overrides the host to which the server should bind.

## Description

Normally, the listener binds to the wildcard host. However, you may want the listener to bind to
another IP instead.

If several addresses are provided, a listener will be open on each of the IP provided.

Each address has to be an IP or name of one of the interfaces of the host. Bind by interface name, binds to the IPs on that interface at the time of startup or reload (reload will happen with a SIGHUP or if the config file changes).

If the given argument is an interface name, and that interface has serveral IP addresses, CoreDNS will listen on all of the interface IP addresses (including IPv4 and IPv6), except for IPv6 link-local addresses on that interface.

## Syntax

In its basic form, a simple bind uses this syntax:

~~~ txt
bind ADDRESS|IFACE  ...
~~~

You can also exclude some addresses with their IP address or interface name in expanded syntax:

~~~
bind ADDRESS|IFACE ... {
    except ADDRESS|IFACE ...
}
~~~



* **ADDRESS|IFACE** is an IP address or interface name to bind to.
When several addresses are provided a listener will be opened on each of the addresses. Please read the *Description* for more details.
* `except`, excludes interfaces or IP addresses to bind to. `except` option only excludes addresses for the current `bind` directive if multiple `bind` directives are used in the same server block.
## Examples

To make your socket accessible only to that machine, bind to IP 127.0.0.1 (localhost):

~~~ corefile
. {
    bind 127.0.0.1
}
~~~

To allow processing DNS requests only local host on both IPv4 and IPv6 stacks, use the syntax:

~~~ corefile
. {
    bind 127.0.0.1 ::1
}
~~~

If the configuration comes up with several *bind* plugins, all addresses are consolidated together:
The following sample is equivalent to the preceding:

~~~ corefile
. {
    bind 127.0.0.1
    bind ::1
}
~~~

The following server block, binds on localhost with its interface name (both "127.0.0.1" and "::1"):

~~~ corefile
. {
    bind lo
}
~~~

You can exclude some addresses by their IP or interface name (The following will only listen on `::1` or whatever addresses have been assigned to the `lo` interface):

~~~ corefile
. {
    bind lo {
        except 127.0.0.1
    }
}
~~~

## Bugs

### Avoiding Listener Contention

TL;DR, When adding the _bind_ plugin to a server block, it must also be added to all other server blocks that listen on the same port.

When more than one server block is configured to listen to a common port, those server blocks must either
all use the _bind_ plugin, or all use default binding (no _bind_ plugin).  Note that "port" here refers the TCP/UDP port that
a server block is configured to serve (default 53) - not a network interface. For two server blocks listening on the same port,
if one uses the bind plugin and the other does not, two separate listeners will be created that will contend for serving
packets destined to the same address.  Doing so will result in unpredictable behavior (requests may be randomly
served by either server). This happens because *without* the *bind* plugin, a server will bind to all
interfaces, and this will collide with another server if it's using *bind* to listen to an address
on the same port. For example, the following creates two servers that both listen on 127.0.0.1:53,
which would result in unpredictable behavior for queries in `a.bad.example.com`:

```
a.bad.example.com {
    bind 127.0.0.1
    forward . 1.2.3.4
}

bad.example.com {
    forward . 5.6.7.8
}
```

Also on MacOS there is an (open) bug where this doesn't work properly. See
<https://github.com/miekg/dns/issues/724> for details, but no solution.
