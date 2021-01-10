# secondary

## Name

*secondary* - enables serving a zone retrieved from a primary server.

## Description

With *secondary* you can transfer (via AXFR) a zone from another server. The retrieved zone is
*not committed* to disk (a violation of the RFC). This means restarting CoreDNS will cause it to
retrieve all secondary zones.

## Syntax

~~~
secondary [ZONES...]
~~~

* **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block
    are used. Note that without a remote address to *get* the zone from, the above is not that useful.

A working syntax would be:

~~~
secondary [zones...] {
    transfer from ADDRESS [ADDRESS...]
}
~~~

*  `transfer from` specifies from which **ADDRESS** to fetch the zone. It can be specified multiple
   times; if one does not work, another will be tried. Transfering this zone outwards again can be
   done by enabling the *transfer* plugin.

When a zone is due to be refreshed (refresh timer fires) a random jitter of 5 seconds is applied,
before fetching. In the case of retry this will be 2 seconds. If there are any errors during the
transfer in, the transfer fails; this will be logged.

## Examples

Transfer `example.org` from 10.0.1.1, and if that fails try 10.1.2.1.

~~~ corefile
example.org {
    secondary {
        transfer from 10.0.1.1 10.1.2.1
    }
}
~~~

Or re-export the retrieved zone to other secondaries.

~~~ corefile
example.net {
    secondary {
        transfer from 10.1.2.1
    }
    transfer {
        to *
    }
}
~~~

## Bugs

Only AXFR is supported and the retrieved zone is not committed to disk.

## See Also

See the *transfer* plugin to enable zone transfers _to_ other servers.
