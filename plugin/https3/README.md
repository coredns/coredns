# https3

## Name

*https3* - configures DNS-over-HTTPS/3 (DoH3) server options.

## Description

The *https3* plugin allows you to configure parameters for the DNS-over-HTTPS/3 (DoH3) server to fine-tune the security posture and performance of the server. HTTPS/3 uses QUIC as the underlying transport.

This plugin can only be used once per HTTPS3 listener block.

## Syntax

```txt
https3 {
    max_streams POSITIVE_INTEGER
}
```

* `max_streams` limits the number of concurrent QUIC streams per connection. This helps prevent unbounded streams on a single connection, exhausting server resources. The default value is 256 if not specified. Set to 0 to use underlying QUIC transport default.

## Examples

Set custom limits for maximum streams:

```
https3://.:443 {
    tls cert.pem key.pem
    https3 {
        max_streams 50
    }
    whoami
}
```

Set values to 0 for QUIC transport default, matching CoreDNS behaviour before v1.14.0:

```
https3://.:443 {
    tls cert.pem key.pem
    https3 {
        max_streams 0
    }
    whoami
}
```
