# view

## Name 

*view* - defines conditions that must be met for a DNS request to be routed to the server block.

## Description

*view* defines an expression that must evaluate to true for a DNS request to be routed to the server block.  
This enables advanced server block routing functions such as split dns.    

### Syntax
```
view  EXPRESSION
```

* `view` **EXPRESSION** - CoreDNS will only route incoming queries to the enclosing server block
  if the **EXPRESSION** evaluates to true. See the **Expressions** section for available variables and functions.
  If multiple instances of view are defined, all **EXPRESSION** must evaluate to true for CoreDNS will only route
  incoming queries to the enclosing server block.


### Examples

The example below implements CIDR based split DNS routing.  It will return a different
answer for `test.` depending on client's IP address.  It returns ...
* `test. 3600 IN A 1.1.1.1`, for queries with a source address in 127.0.0.0/24
* `test. 3600 IN A 2.2.2.2`, for queries with a source address in 192.168.0.0/16
* `test. 3600 IN A 3.3.3.3`, for all others

```
. {
  view incidr(client_ip, '127.0.0.0/24')
  hosts {
    1.1.1.1 test
  }
}

. {
  view incidr(client_ip, '192.168.0.0/16')
  hosts {
    2.2.2.2 test
  }
}

. {
  hosts {
    3.3.3.3 test
  }
}
```

## Expressions

### Available Variables

* `type`: type of the request (A, AAAA, TXT, ...)
* `name`: name of the request (the domain name requested)
* `class`: class of the request (IN, CH, ...)
* `proto`: protocol used (tcp or udp)
* `client_ip`: client's IP address, for IPv6 addresses these are enclosed in brackets: `[::1]`
* `size`: request size in bytes
* `port`: client's port
* `bufsize`: the EDNS0 buffer size advertised in the query
* `do`: the EDNS0 DO (DNSSEC OK) bit set in the query
* `id`: query ID
* `opcode`: query OPCODE
* `server_ip`: server's IP address; for IPv6 addresses these are enclosed in brackets: `[::1]`
* `server_port` : client's port

#### Metadata Variables

Metadata variables are not currently supported in expressions for this plugin.

### Available Functions

* `incidr(ip,cidr)`: returns true if _ip_ is within _cidr_ 

