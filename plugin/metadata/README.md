# metadata

## Name

*metadata* - enables a metadata collector. Allows to add metadata for DNS request based on pre-defined
mapping.

## Description

By enabling *metadata* any plugin that implements [metadata.Provider
interface](https://godoc.org/github.com/coredns/coredns/plugin/metadata#Provider) will be called for
each DNS query, at the beginning of the process for that query, in order to add its own metadata to
context.

The metadata collected will be available for all plugins, via the Context parameter provided in the
ServeDNS function. The package (code) documentation has examples on how to inspect and retrieve
metadata a plugin might be interested in.

The metadata is added by setting a label with a value in the context. These labels should be named
`plugin/NAME`, where **NAME** is something descriptive. The only hard requirement the *metadata*
plugin enforces is that the labels contain a slash. See the documentation for
`metadata.SetValueFunc`.

The value stored is a string. The empty string signals "no metadata". See the documentation for
`metadata.ValueFunc` on how to retrieve this.

Additionally, the `map` setting can be used to define mappings from DNS request fields (such as client_ip, name or type)
to specific metadata label-value pairs. These pairs are then stored in context and can be used in other plugins.

## Syntax

~~~
metadata [ZONES... ]
~~~

* **ZONES** zones metadata should be invoked for.

You can further specify the mapping of request field to specific metadata using `map`:

~~~
metadata [ZONES... ] {
    map <source>  <labels...> {
        <input>   <values...>
        [default] [rcode|defaults...]
    }
}
~~~

* source - is the source of input value to switch on. It is a Go template used to evaluate expressions based on the 
[Request structure](https://pkg.go.dev/github.com/coredns/coredns/request#Request).
* input - is the input value to match source.
* labels - metadata labels.
* values - metadata values to store in the associated metadata labels.
* default - defines default behavior if none of the inputs are satisfied. If omitted, the next plugin will simply be
  invoked.
* rcode -  a response code (`NXDOMAIN, SERVFAIL, ...`) that will be returned in default case. Valid response code values
  are per the `RcodeToString` map defined by the `miekg/dns` package in `msg.go`.
* defaults - default metadata values.

## Plugins

`metadata.Provider` interface needs to be implemented by each plugin willing to provide metadata
information for other plugins. It will be called by metadata and gather the information from all
plugins in context.

Note: this method should work quickly, because it is called for every request.

## Examples

Enable metadata collector:

~~~ corefile
. {
    metadata
}
~~~

Enable metadata collection for a specific zone:

~~~ corefile
. {
    metadata example.com.
}
~~~

Define a mapping using the client IP as the source:
- For requests from IP 127.0.0.1, the metadata `metadata/VRF` is set to `VRF1` and `metadata/comment` to `local`.
- For client IPs not defined in the mapping, the default section is invoked.

~~~ corefile
. {
    metadata {
        map {{.IP}}   VRF  comment {
            127.0.0.1 VRF1 local
            ::1       VRF2 "local IPv6"
            default   VRF3 default
        }
    }
}
~~~

Define a mapping without a default, using the request type as the source. 
In this scenario, if the request type is PTR, no metadata will be set in the context.

~~~ corefile
. {
    metadata {
        map "{{ .Type }}" server {
            A             8.8.8.8
            AAAA          8.8.4.4
        }
    }
}
~~~

Define a mapping with a default response of SERVFAIL. 
Here, if the request type is PTR, a SERVFAIL response will be returned.

~~~ corefile
. {
    metadata {
        map "{{ .Type }}" server   port {
            A             8.8.8.8  53
            AAAA          8.8.4.4  53
            default       SERVFAIL
        }
    }
}
~~~

## See Also

* [Provider interface](https://godoc.org/github.com/coredns/coredns/plugin/metadata#Provider) documentation
* [Package level](https://godoc.org/github.com/coredns/coredns/plugin/metadata) documentation
* [Go template](https://golang.org/pkg/text/template/) for the template language reference
