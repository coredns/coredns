# alternate

## Name

Plugin *Alternate* is able to selectively forward the query to another upstream server, depending the error result provided by the initial resolver

## Description

The *alternate* plugin allows an alternate set of upstreams be specified which will be used
if the plugin chain returns specific error messages. The *alternate* plugin utilizes the *forward* plugin (<https://coredns.io/plugins/forward>) to query the specified upstreams.

As the name suggests, the purpose of the *alternate* is to allow a alternate when, for example,
the desired upstreams became unavailable.

## Syntax

```
{
    alternate [original] RCODE FORWARD_PARAMS
}
```

* **original** is optional flag. If it is set then alternate uses original request instead of potentially changed by other plugins
* **RCODE** is the string representation of the error response code. The complete list of valid rcode strings are defined as `RcodeToString` in <https://github.com/miekg/dns/blob/master/msg.go>, examples of which are `SERVFAIL`, `NXDOMAIN` and `REFUSED`.
* **FORWARD_PARAMS** accepts the same parameters as the *forward* plugin
<https://coredns.io/plugins/forward>.

## Examples

### Alternate to local DNS server

The following specifies that all requests are forwarded to 8.8.8.8. If the response is `NXDOMAIN`, *alternate* will forward the request to 192.168.1.1:53, and reply to client accordingly.

```
. {
	forward . 8.8.8.8
	alternate NXDOMAIN . 192.168.1.1:53
	log
}

```
### Alternate with original request used

The following specify that `original` query will be forwarded to 192.168.1.1:53 if 8.8.8.8 response is `NXDOMAIN`. `original` means no changes from next plugins on request. With no `original` flag alternate will forward request with EDNS0 option (set by rewrite).

```
. {
	forward . 8.8.8.8
    rewrite edns0 local set 0xffee 0x61626364
	alternate original NXDOMAIN . 192.168.1.1:53
	log
}

```

### Multiple alternates

Multiple alternates can be specified, as long as they serve unique error responses.

```
. {
    forward . 8.8.8.8
    alternate NXDOMAIN . 192.168.1.1:53
    alternate REFUSED . 192.168.100.1:53
    alternate original SERVFAIL . 192.168.100.1:53
    log
}

```

### Additional forward parameters

You can specify additional forward parameters for each of the alternate upstreams.

```
. {
    forward . 8.8.8.8
    alternate NXDOMAIN . 192.168.1.1:53 192.168.1.2:53 {
        max_fails 5
        force_tcp
    }
    log
}
```
