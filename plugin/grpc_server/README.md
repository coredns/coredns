# grpc_server

## Name

*grpc_server* - configures DNS-over-gRPC server options.

## Description

The *grpc_server* plugin allows you to configure parameters for the DNS-over-gRPC server to fine-tune the security posture and performance of the server.

This plugin can only be used once per gRPC listener block.

## Syntax

```txt
grpc_server {
    max_streams POSITIVE_INTEGER
    max_connections POSITIVE_INTEGER
}
```

* `max_streams` limits the number of concurrent gRPC streams per connection. This helps prevent unbounded streams on a single connection, exhausting server resources. The default value is 256 if not specified. Set to 0 for unbounded.
* `max_connections` limits the number of concurrent TCP connections to the gRPC server. The default value is 200 if not specified. Set to 0 for unbounded.

## Examples

Set custom limits for maximum streams and connections:

```
grpc://.:8053 {
    tls cert.pem key.pem
    grpc_server {
        max_streams 50
        max_connections 100
    }
    whoami
}
```

Set values to 0 for unbounded, matching CoreDNS behaviour before v1.14.0:

```
grpc://.:8053 {
    tls cert.pem key.pem
    grpc_server {
        max_streams 0
        max_connections 0
    }
    whoami
}
```
