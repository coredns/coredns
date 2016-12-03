# whoareyou

whoareyou returns dns server IP address, port and transport used. Server IP address is returned in
the answer section as either an A or AAAA record.

The port and transport are included in the additional section as a SRV record, transport can be
"tcp" or "udp".

~~~ txt
._<transport>.qname. 0 IN SRV 0 0 <port> .
~~~

## Syntax

~~~ txt
whoareyou
~~~

## Examples

~~~ txt
.:53 {
    whoareyou
}
~~~

When queried for "example.org A", CoreDNS will respond with:

~~~ txt
;; QUESTION SECTION:
;example.org.              IN  A

;; ANSWER SECTION:
example.org.            0  IN  A    127.0.0.1

;; ADDITIONAL SECTION:
_udp.example.org.       0  IN  SRV  0 0 53
~~~

When queried for "example.org AAAA", CoreDNS will respond with:

~~~ txt
;; QUESTION SECTION:
;example.org.              IN  AAAA

;; ANSWER SECTION:
example.org.            0  IN  AAAA   ::1

;; ADDITIONAL SECTION:
_udp.example.org.       0  IN  SRV    0 0 53
~~~
