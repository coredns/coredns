# acme
## Name
*acme* - Performs the ACME protocol for the specified domain

## Description
The *acme* plugin is useful for automating certificate management through the `ACME`.

Make sure that:
* You own the domain
* Your CoreDNS server is the authoritative nameserver for the domain

## Syntax
~~~ txt
acme {
  domain <DOMAIN>

  # optional parameters
  challenge <CHALLENGE> port <PORT>
}
~~~

* `DOMAIN` is the domain name the plugin should be authoritative for.
* By default, the **DNS** challenge will be used for ACME.

You can specify one or more challenges the CA can use to verify your ownership of the domain.
* `CHALLENGE` is the name of the challenge you will use for ACME. There are only two options: `tlsalpn` and `http01`.
* `PORT` is the port number to use for each challenge. Make sure the ports are open and accessible.

## Examples
### Basic
~~~ corefile
acme {
  domain example.org
}
~~~
This will perform ACME for `example.org` and use the `DNS01` challenge only.

### Advanced
~~~ corefile
acme {
  domain example.com

  challenge http port 90
  challenge tlsalpn port 8080
}
~~~
This will perform ACME for `example.com` and perform the following challenges:
1. `HTTP` challenge on port **90**
2. `TLSALPN` challenge on port **8080**
3. `DNS` challenge

## See Also
1. [RFC for ACME](https://datatracker.ietf.org/doc/html/rfc8555/)
2. [ACME Protocol](https://www.thesslstore.com/blog/acme-protocol-what-it-is-and-how-it-works/)
3. [Challenge Types](https://letsencrypt.org/docs/challenge-types/)
