# fallback

## Name

*fallback* - send failed DNS queries to fallback endpoints.

## Description

This plugin sends failed DNS quieres (e.g., NXDOMAIN, SERVFAIL, etc)
to fallback endpoints.

## Syntax

~~~ txt
fallback [ZONE] {
  on [FAILURE] [ENDPOINTS...]
}
~~~

* **ZONE** the name of the domain to be accessed.
* **FAILURE** the failure code of the DNS queries (NXDOMAIN, SERVFAIL, etc.).
* **ENDPOINTS** the fallback endpoints to send to.

## Examples

~~~ corefile
. {
    fallback example.org {
      on NXDOMAIN 10.10.10.10:53
    }
}
~~~
