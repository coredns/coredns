# tls

## Name

*tls* - allows you to configure the server certificates for the TLS, gRPC, DoH servers.

## Description

CoreDNS supports queries that are encrypted using TLS (DNS over Transport Layer Security, RFC 7858)
or are using gRPC (https://grpc.io/ , not an IETF standard). Normally DNS traffic isn't encrypted at
all (DNSSEC only signs resource records).

The *tls* "plugin" allows you to configure the cryptographic keys that are needed for both
DNS-over-TLS and DNS-over-gRPC. If the *tls* plugin is omitted, then no encryption takes place.

The gRPC protobuffer is defined in `pb/dns.proto`. It defines the proto as a simple wrapper for the
wire data of a DNS message.

## Syntax

~~~ txt
tls CERT KEY [CA]
~~~

Parameter CA is optional. If not set, system CAs can be used to verify the client certificate

~~~ txt
tls CERT KEY [CA] {
    client_auth nocert|request|require|verify_if_given|require_and_verify
    keylog FILE
}
~~~

If client\_auth option is specified, it controls the client authentication policy.
The option value corresponds to the [ClientAuthType values of the Go tls package](https://golang.org/pkg/crypto/tls/#ClientAuthType): NoClientCert, RequestClientCert, RequireAnyClientCert, VerifyClientCertIfGiven, and RequireAndVerifyClientCert, respectively.
The default is "nocert".  Note that it makes no sense to specify parameter CA unless this option is
set to verify\_if\_given or require\_and\_verify.

The keylog can be specified to export TLS master secrets in key log format to allow external programs
to decrypt TLS connections. It compromises security and should only be used for debugging!

CoreDNS sets the minimum TLS version to TLS 1.2. The maximum TLS version, TLS 1.2 cipher suites, and
key exchange mechanisms use the Go `crypto/tls` defaults.

Certificates can instead be obtained and renewed automatically with ACME:

~~~ txt
tls {
    acme DOMAIN...
    email EMAIL
    ca URL
    storage DIRECTORY
    ca_root FILE
    resolver ADDRESS
}
~~~

The `acme` property enables automatic certificate management for one or more domain names. CoreDNS
uses the DNS-01 challenge and answers the temporary `_acme-challenge` TXT queries on every DNS
listener in the same CoreDNS instance. The domains' authoritative DNS must therefore reach this
CoreDNS instance over port 53. HTTP-01 and TLS-ALPN-01 challenges are not used.

The remaining properties are optional:

* `email` sets the ACME account contact address.
* `ca` sets the ACME directory URL. It defaults to the Let's Encrypt production directory.
* `storage` sets the directory for ACME accounts, certificates, and private keys. It defaults to
  `.coredns/acme` below the Corefile root.
* `ca_root` adds a PEM certificate bundle for connecting to a private ACME server.
* `resolver` sets the DNS resolver used to reach the ACME server and must use `HOST:PORT` syntax.

Certificate management starts in the background after all listeners are active. A new encrypted
listener can reject TLS handshakes until its first certificate has been obtained. Renewed certificates
are used without restarting CoreDNS.

The DNS-01 challenge state is local to one CoreDNS process. When authoritative DNS is served by
multiple replicas, validation queries must be routed to the replica performing the ACME operation.

## Examples

Start a DNS-over-TLS server that picks up incoming DNS-over-TLS queries on port 5553 and uses the
nameservers defined in `/etc/resolv.conf` to resolve the query. This proxy path uses plain old DNS.

~~~
tls://.:5553 {
	tls cert.pem key.pem ca.pem
	forward . /etc/resolv.conf
}
~~~

Start a DNS-over-gRPC server that is similar to the previous example, but using DNS-over-gRPC for
incoming queries.

~~~
grpc://. {
	tls cert.pem key.pem ca.pem
	forward . /etc/resolv.conf
}
~~~

Start a DoH server on port 443 that is similar to the previous example, but using DoH for incoming queries.
~~~
https://. {
	tls cert.pem key.pem ca.pem
	forward . /etc/resolv.conf
}
~~~

Obtain and renew a certificate for a DoT server. The plain DNS server answers the DNS-01 challenge;
both server blocks must be in the same CoreDNS process.

~~~
.:53 {
	file example.org
}

tls://.:853 {
	tls {
		acme dns.example.org
		email hostmaster@example.org
	}
	forward . /etc/resolv.conf
}
~~~

Only Knot DNS' `kdig` supports DNS-over-TLS queries, no command line client supports gRPC making
debugging these transports harder than it should be.

## See Also

RFC 7858 and https://grpc.io.
