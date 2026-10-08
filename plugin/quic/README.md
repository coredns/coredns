# quic

## Name

*quic* - configures DNS-over-QUIC (DoQ) server options.

## Description

The *quic* plugin allows you to configure parameters for the DNS-over-QUIC (DoQ) server to fine-tune the security posture and performance of the server.

This plugin can only be used once per quic Server Block.

## Syntax

```txt
quic {
    max_streams POSITIVE_INTEGER
    max_connections NON_NEGATIVE_INTEGER
    worker_pool_size POSITIVE_INTEGER
}
```

* `max_streams` limits the number of concurrent QUIC streams per connection. This helps prevent DoS attacks where an attacker could open many streams on a single connection, exhausting server resources. The default value is 256 if not specified.
* `max_connections` limits the number of concurrent QUIC connections accepted by the server. The default value is 200 if not specified. Connections above the configured limit are rejected. Set to 0 to disable the CoreDNS connection limit.
* `worker_pool_size` defines the size of the worker pool for processing QUIC streams across all connections. The default value is 512 if not specified. This limits the total number of concurrent streams that can be processed across all connections.

## Examples

Enable DNS-over-QUIC with default settings (256 concurrent streams per connection, 200 concurrent connections, 512 worker pool size):

```
quic://.:8853 {
    tls cert.pem key.pem
    quic
    whoami
}
```

Set custom limits for maximum QUIC streams per connection, maximum connections, and worker pool size:

```
quic://.:8853 {
    tls cert.pem key.pem
    quic {
        max_streams 16
        max_connections 50
        worker_pool_size 65536
    }
    whoami
}
```
