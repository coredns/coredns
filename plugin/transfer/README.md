# transfer

## Name

*transfer* - perform zone transfers for other plugins.

## Description

This plugin answers zone transfers for authoritative plugins that implement
`transfer.Transferer`.  Currently, no internal plugins implement this interface.

Transfer answers full zone transfer (AXFR) requests and incremental zone transfer (IXFR) requests
with AXFR fallback if the zone has changed.

Notifies are sent to all hosts in `to` fields when the zone changes.

## Syntax

~~~
transfer [ZONE...] {
  to HOST-IP [notify [source SOURCE-IP]]
}
~~~

* **ZONES** The zones *transfer* will answer zone requests for. If left blank,
  the zones are inherited from the enclosing server block. To answer zone
  transfers for a given zone, there must be another plugin in the same server
  block that serves the same zone, and implements `transfer.Transferer`.

* `to ` **HOST-IP** The host *transfer* will transfer to. Use `*` to permit
  transfers to all hosts. If `notify` is included, notifies will be sent
  to the host. The `to` option may be specified more than once to
  define multiple hosts.  The `source SOURCE-IP` option controls which
  interface will be used when sending notifies to the host.

## Examples

TODO
