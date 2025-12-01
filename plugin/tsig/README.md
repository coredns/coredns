# tsig

## Name

*tsig* - define TSIG keys, validate incoming TSIG signed requests and sign responses.

## Description

With *tsig*, you can define CoreDNS's TSIG secret keys. Using those keys, *tsig* validates incoming TSIG requests and signs
responses to those requests. It does not itself sign requests outgoing from CoreDNS; it is up to the
respective plugins sending those requests to sign them using the keys defined by *tsig*.

The *tsig* plugin can also require that incoming requests be signed for certain query types, refusing requests that do not comply.

## Syntax

~~~
tsig [ZONE...] {
  secret NAME KEY
  secrets FILE
  require [QTYPE...]
}
~~~

   * **ZONE** - the zones *tsig* will TSIG.  By default, the zones from the server block are used.

   * `secret` **NAME** **KEY** - specifies a TSIG secret for **NAME** with **KEY**. Use this option more than once
   to define multiple secrets. Secrets are global to the server instance, not just for the enclosing **ZONE**.

   * `secrets` **FILE** - same as `secret`, but load the secrets from a file. The file may define any number
     of unique keys, each in the following `named.conf` format:
     ```cgo
     key "example." {
         secret "X28hl0BOfAL5G0jsmJWSacrwn7YRm2f6U5brnzwWEus=";
     };
     ```
     Each key may also specify an `algorithm` e.g. `algorithm hmac-sha256;`, but this is currently ignored by the plugin.

     * `require` **QTYPE...** - the query types that must be TSIG'd. Requests of the specified types
   will be `REFUSED` if they are not signed.`require all` will require requests of all types to be
   signed. `require none` will not require requests any types to be signed. Default behavior is to not require.

## Examples

Require TSIG signed transactions for transfer requests to `example.zone`.

```
example.zone {
  tsig {
    secret example.zone.key. NoTCJU+DMqFWywaPyxSijrDEA/eC3nK0xi3AMEZuPVk=
    require AXFR IXFR
  }
  transfer {
    to *
  }
}
```

Require TSIG signed transactions for all requests to `auth.zone`.

```
auth.zone {
  tsig {
    secret auth.zone.key. NoTCJU+DMqFWywaPyxSijrDEA/eC3nK0xi3AMEZuPVk=
    require all
  }
  forward . 10.1.0.2
}
```

## Bugs

### Secondary

TSIG transfers are not yet implemented for the *secondary* plugin.  The *secondary* plugin will not sign its zone transfer requests.

### Zone Transfer Notifies

With the *transfer* plugin, zone transfer notifications from CoreDNS are not TSIG signed.

### Special Considerations for Forwarding Servers (RFC 8945 5.5)

https://datatracker.ietf.org/doc/html/rfc8945#section-5.5

CoreDNS does not implement this section as follows ...

* RFC requirement:
  > If the name on the TSIG is not
of a secret that the server shares with the originator, the server
MUST forward the message unchanged including the TSIG.

  CoreDNS behavior:
If ths zone of the request matches the _tsig_ plugin zones, then the TSIG record
is always stripped. But even when the _tsig_ plugin is not involved, the _forward_ plugin
may alter the message with compression, which would cause validation failure
at the destination.


* RFC requirement:
  > If the TSIG passes all checks, the forwarding
server MUST, if possible, include a TSIG of its own to the
destination or the next forwarder.

  CoreDNS behavior:
If ths zone of the request matches the _tsig_ plugin zones, _forward_ plugin will
proxy the request upstream without TSIG.


* RFC requirement:
  > If no transaction security is
available to the destination and the message is a query, and if the
corresponding response has the AD flag (see RFC4035) set, the
forwarder MUST clear the AD flag before adding the TSIG to the
response and returning the result to the system from which it
received the query.

  CoreDNS behavior:
The AD flag is not cleared.
