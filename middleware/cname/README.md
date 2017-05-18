# cname

`cname` enables CNAME flattening. See [this blog post from
Cloudflare](https://blog.cloudflare.com/introducing-cname-flattening-rfc-compliant-cnames-at-a-domains-root/)
on what CNAME flattening is. With this middleware enabled *any* CNAME will be resolved and
substituted in the answer.

## Syntax

~~~
cname [zones...] [upstream [address...]]
~~~

* `zones` zones that should be flattened. If empty the zones from the configuration block
    are used.
* `address` upstream server(s) used for resolving names. If not specified 8.8.8.8:53 and 8.8.4.4:53
    are used.

## Examples

~~~
cname upstream 127.0.0.1:53
~~~
