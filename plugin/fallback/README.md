# fallback

## Name

*fallback* - a plugin that sends queries to an alternate set of upstreams if the plugin
chain returns specific error messages.

## Description

The *fallback* plugin allows an alternate set of upstreams be specified which will be used
if the plugin chain returns specific error messages. The *fallback* plugin utilizes the *proxy*
plugin (<https://github.com/coredns/coredns/tree/master/plugin/proxy>) to query the specified
upstreams.

As the name suggests, the purpose of the *fallback* is to allow a fallback when, for example, the
desired upstreams became unavailable.

## Syntax

```
{
    fallback RCODE PROXY_PARAMS
}
```

* `RCODE` is the string representation of the error response code. The set of valid error
strings are defined as `RcodeToString` in <https://github.com/miekg/dns/blob/master/msg.go>
* `PROXY_SPECS` accepts the same parameters as the *proxy* plugin
<https://github.com/coredns/coredns/tree/master/plugin/proxy>.

## Examples

### Fallback to local DNS server

The following specifies that all requests are proxied to 8.8.8.8. If the response is `NXDOMAIN`, the
fallback will proxy the request to 192.168.1.1:53, and reply to client according.

```
. {
	proxy . 8.8.8.8
	fallback NXDOMAIN . 192.168.1.1:53
	log
}

```

### Multiple fallbacks

Multiple fallback can be specified, as long as they serve unique error responses.

```
. {
    proxy . 8.8.8.8
    fallback NXDOMAIN . 192.168.1.1:53
    fallback REFUSED . 192.168.100.1:53
    log
}

```

### Additional proxy parameters

You can specify additional proxy parameters for each of the fallback upstreams.

```
. {
    proxy . 8.8.8.8
    fallback NXDOMAIN . 192.168.1.1:53 192.168.1.2:53 {
        protocol dns force_tcp
    }
    log
}
```
