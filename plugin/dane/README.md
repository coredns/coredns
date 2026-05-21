
# dane

## Name

*dane* - generates TLSA responses from certificate files.

## Description

*dane* simplifies rotating TLS certificates that are served over DNS.
An upstream plugin such as *file* can return a dummy value in certificate association data
which is then replaced by the real value computed from the PEM certificate file.

## Syntax

~~~ txt
dane [ZONES...] {
    file KEY PATH
}
~~~

* **ZONES** zones that should have their responses rewritten. If empty, the zones from the
configuration block are used.
* `file` indicates that the **KEY** association data should be replaced by association data
computed from **FILE**. This directive can be specified more than once.

## Examples

Respond to `_25._tcp.example.org` with a TLSA response using data from `fullchain.pem`
~~~ txt
example.org {
    records {
        _25._tcp 60 IN TLSA 2 0 1 EE
    }
    dane {
        file EE fullchain.pem
    }
}
~~~
