# https

## Name

*https* - configures DNS-over-HTTPS (DoH) server options.

## Description

The *https* plugin allows you to configure parameters for the DNS-over-HTTPS (DoH) server to fine-tune the security posture and performance of the server.

This plugin can only be used once per HTTPS listener block.

## Syntax

```txt
https {
    max_connections NON_NEGATIVE_INTEGER
    max_streams NON_NEGATIVE_INTEGER
}
```

* `max_connections` limits the number of concurrent TCP connections to the HTTPS server. The default value is 200 if not specified. Set to 0 for unbounded.
* `max_streams` limits the number of concurrent HTTP/2 streams per HTTPS connection. This helps prevent unbounded streams on a single connection, exhausting server resources. The default value is 250 if not specified. Set to 0 to use the underlying HTTP/2 transport default.

## Examples

Set custom limits for maximum connections and streams:

```
https://.:443 {
    tls cert.pem key.pem
    https {
        max_connections 100
        max_streams 100
    }
    whoami
}
```

Set both values to 0 to disable the CoreDNS limits (unbounded connections and the underlying HTTP/2 transport stream default), matching CoreDNS behaviour before v1.14.0:

```
https://.:443 {
    tls cert.pem key.pem
    https {
        max_connections 0
        max_streams 0
    }
    whoami
}
```
