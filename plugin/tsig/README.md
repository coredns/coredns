# tsig

## Name

*tsig* - validate TSIG requests and sign responses.

## Description

With *tsig*, you can define a set of TSIG secret keys for validating incoming TSIG requests and signing
responses. It can also require TSIG for certain query types, refusing requests that do not comply.

## Syntax

~~~
tsig [ZONE...] {
  secret NAME KEY
  require [QTYPE...]
}
~~~

   * **ZONE** - the zones *tsig* will TSIG.  By default, the zones from the server block are used.

   * `secret` **KEY** - specifies a TSIG secret for **NAME** with **KEY**. Use this option more than once
   to define multiple secrets. Secrets are global to the server instance, not just for the enclosing **ZONE**.

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
