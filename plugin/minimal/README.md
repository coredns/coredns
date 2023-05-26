# minimal

## Name

*minimal* - minimizes size of the DNS response message whenever possible.

## Description

The *minimal* plugin tries to minimize the size of the response. Depending on the response type it
removes resource records from the AUTHORITY and ADDITIONAL sections.

Specifically this plugin looks at successful responses (this excludes negative responses, i.e.
nodata or name error). If the successful response isn't a delegation only the RRs in the answer
section are written to the client.

## Syntax

~~~ txt
minimal
~~~

## Examples

Enable minimal responses:

~~~ corefile
example.org {
    whoami
    forward . 8.8.8.8
    minimal
}
~~~

## See Also

[BIND 9 Configuration Reference](https://bind9.readthedocs.io/en/latest/reference.html#boolean-options)
