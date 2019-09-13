# transfer

## Name

*transfer* - answer zone transfers requests for compatible authoritative
plugins.

## Description

This plugin answers zone transfers for authoritative plugins that implement
`transfer.Transferer`.

Transfer answers AXFR requests and IXFR requests with AXFR fallback if the
zone has changed.

Notifies are not supported.

*transfer* is not currently implemented by any plugins.

## Syntax

~~~
transfer [ZONE...] {
  to HOST...
}
~~~

* **ZONES** The zones *transfer* will answer zone requests for. If left blank,
  the zones are inherited from the enclosing server block. To answer zone
  transfers for a given zone, there must be another plugin in the same server
  block that serves the same zone, and implements `transfer.Transferer`.

* `to ` **HOST...** The hosts *transfer* will transfer to. Use `*` to permit
  transfers to all hosts.

# Examples

Answer zone transfers for `example.com.` using the theoretical plugin *plugin1*.

~~~
example.com {
    transfer {
      to *
    }
    plugin2
}

~~~

You can enable zone transfer for more than one plugin with a single *transfer* instance.
Here, *transfer* will send zone transfers to any source for the theoretical plugins *plugin1*
and *plugin2* (both of which must both implement `transfer.Transferer`).

~~~
. {
    transfer a.example.com b.example.com{
      to *
    }
    plugin1 a.example.com
    plugin2 b.example.com
}

~~~

Building on the example above, you can use *transfer* more than once in a server block to specify
separate options per zone.

~~~
. {
    transfer a.example.com {
      to *
    }
    transfer b.example.com {
      to 192.168.0.100
    }
    plugin1 a.example.com
    plugin2 b.example.com
}

~~~