# https

## Name

*https* - configures DNS-over-HTTPS (DoH) server options.

## Description

The *https* plugin allows you to configure parameters for the DNS-over-HTTPS (DoH) server to fine-tune the security posture and performance of the server.

This plugin can only be used once per HTTPS listener block.

## Syntax

```txt
https {
    max_connections POSITIVE_INTEGER
}
```

* `max_connections` limits the number of concurrent TCP connections to the HTTPS server. The default value is 200 if not specified. Set to 0 for unbounded.

## Examples

Set custom limits for maximum connections:

```
https://.:443 {
    tls cert.pem key.pem
    https {
        max_connections 100
    }
    whoami
}
```

Set values to 0 for unbounded, matching CoreDNS behaviour before v1.14.0:

```
https://.:443 {
    tls cert.pem key.pem
    https {
        max_connections 0
    }
    whoami
}
```
