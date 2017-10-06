# forward

*forward* facilitates proxying DNS messages to upstream resolvers.

The *forward* plugin is generally faster (~30%) than *proxy* as it re-uses already openened sockets to the
upstreams. It supports UDP and TCP and uses inband healthchecking that is enabled by default.

## Syntax

In its most basic form, a simple forwarder uses this syntax:

~~~
forward FROM TO...
~~~

* **FROM** is the base domain to match for the request to be forwared.
* **TO...** are the destination endpoints to forward to.

By default health checks are done every 0.5s. After two failed checks the upstream is
considered unhealthy. The health checks use a non-recursive DNS query (`. IN NS`) to get upstream
health. Any reponse that is not an error is taken as a healthy upstream. Multi upstreams are
randomized on first use. Note that when a healthy upstream fails to respond to a query this error
is propegated to the client and no other upstream is tried.

Extra knobs are available with an expanded syntax:

~~~
proxy FROM TO... {
    except IGNORED_NAMES...
    force_tcp
    health_check DURATION
    max_fails INTEGER
}
~~~

* **FROM** and **TO...** as above.
* **IGNORED_NAMES** in `except` is a space-separated list of domains to exclude from proxying.
  Requests that match none of these names will be passed through.
* `force_tcp`, use TCP even when the request comes in over UDP.
* `health_checks`, use a different **DURATION** for health checking, the default duration is 500ms.
* `max_fails` is the number of subsequent failed health checks that are needed before considering
  a backend to be down. If 0, the backend will never be marked as down. Default is 2.

## Metrics

If monitoring is enabled (via the *prometheus* directive) then the following metric are exported:

* `coredns_forward_request_duration_millisecond{proto, family, to}` - duration per upstream
  interaction.
* `coredns_forward_request_count_total{proto, family, to}` - query count per upstream.
* `coredns_forward_socket_count_total{to}` - number of known sockets per upstream.

Where `to` is one of the upstream servers (**TO** from the config), `proto` is the protocol used by
the incoming query ("tcp" or "udp"), and family the transport family ("1" for IPv4, and "2" for
IPv6).

## Examples

Proxy all requests within example.org. to a nameserver running on a different port:

~~~ corefile
example.org {
    forward . 127.0.0.1:9005
}
~~~

Load-balance all requests between three resolvers:

~~~ corefile
. {
    forward . 10.0.0.10:53 10.0.0.11:1053 10.0.0.12
}
~~~

Forward everything except requests to `miek.nl` or `example.org`

~~~ corefile
. {
    forward . 10.0.0.10:1234 {
        except miek.nl example.org
    }
}
~~~

Proxy everything except `example.org` using the host's `resolv.conf`'s nameservers:

~~~ corefile
. {
    forward . /etc/resolv.conf {
        except example.org
    }
}
~~~

Forward to a IPv6 host:

~~~ corefile
. {
    forward . [::1]:1053
}
~~~
