# transfer

## Name

*transfer* - perform (outgoing) zone transfers for other plugins.

## Description

This plugin answers zone transfers for authoritative plugins that implement `transfer.Transferer`.

*transfer* answers full zone transfer (AXFR) requests and incremental zone transfer (IXFR) requests
with AXFR fallback if the zone has changed.

When a plugin wants to notify it's secondaries it will call back into the *transfer* plugin.

The following plugins implement zone transfers using this plugin: *file*, *auto*, *secondary*, and
*kubernetes*. See `transfer.go` for implementation details if you are a plugin author that wants to
use this plugin.

## Syntax

~~~
transfer [ZONE...] {
  to ADDRESS...
}
~~~

 *  **ZONE** The zones *transfer* will answer zone transfer requests for. If left blank, the zones
    are inherited from the enclosing server block. To answer zone transfers for a given zone,
    there must be another plugin in the same server block that serves the same zone, and implements
    `transfer.Transferer`.

 *  `to` **ADDRESS...** The hosts *transfer* will transfer to. Use `*` to permit transfers to all
    addresses. Zone change notifications are sent to all **ADDRESS** that are an IP address or
    an IP address and port e.g. `1.2.3.4`, `12:34::56`, `1.2.3.4:5300`, `[12:34::56]:5300`.
    `to` may be specified multiple times.

You can use the _acl_ plugin to further restrict hosts permitted to receive a zone transfer.
See example below.

## Examples

Use in conjunction with the _acl_ plugin to restrict access to subnet 10.1.0.0/16.

```
...
  acl {
    allow type AXFR net 10.1.0.0/16
    allow type IXFR net 10.1.0.0/16
    block type AXFR net *
    block type IXFR net *
  }
  transfer {
    to *
  }
...
```

Each plugin that can use _transfer_ includes an example of use in their respective documentation.