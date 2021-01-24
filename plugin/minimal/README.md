# minimal

## Name

*minimal* - minimizes size of the DNS response message whenever possible.

## Description

The *minimal* plugin tries to minimize the size of the response. Depending on the response type it removes resource records from the AUTHORITY and ADDITIONAL sections.


## Syntax

~~~ txt
minimal
~~~

## Examples

Enable minimal responses:

~~~ corefile
example.org {
    whoami
    minimal
}
~~~

## See Also

[BIND 9 Configuration Reference](https://bind9.readthedocs.io/en/latest/reference.html#boolean-options)
