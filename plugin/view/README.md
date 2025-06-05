# view

## Name

*view* - defines conditions that must be met for a DNS request to be routed to the server block.

## Description

*view* defines an expression that must evaluate to true for a DNS request to be routed to the server block.
This enables advanced server block routing functions such as split dns.

## Syntax
```
view NAME {
  expr EXPRESSION
}
```

* `view` **NAME** - The name of the view used by metrics and exported as metadata for requests that match the
  view's expression
* `expr` **EXPRESSION** - CoreDNS will only route incoming queries to the enclosing server block
  if the **EXPRESSION** evaluates to true. See the **Expressions** section for available variables and functions.
  If multiple instances of view are defined, all **EXPRESSION** must evaluate to true for CoreDNS will only route
  incoming queries to the enclosing server block.

For expression syntax and examples, see the Expressions and Examples sections.

## Examples

Implement CIDR based split DNS routing.  This will return a different
answer for `test.` depending on client's IP address.  It returns ...
* `test. 3600 IN A 1.1.1.1`, for queries with a source address in 127.0.0.0/24
* `test. 3600 IN A 2.2.2.2`, for queries with a source address in 192.168.0.0/16
* `test. 3600 IN AAAA 2001:0DB8::1`, for queries with a source address in 2001:0DB8::/32
* `test. 3600 IN A 3.3.3.3`, for all others

```
. {
  view example1 {
    expr incidr(client_ip(), '127.0.0.0/24')
  }
  hosts {
    1.1.1.1 test
  }
}

. {
  view example2 {
    expr incidr(client_ip(), '192.168.0.0/16')
  }
  hosts {
    2.2.2.2 test
  }
}

. {
  view v6_example1 {
    expr incidr(client_ip(), '2001:0DB8::/32')
  }
  hosts {
    2001:0DB8::1 test
  }
}

}

. {
  hosts {
    3.3.3.3 test
  }
}
```

Send all `A` and `AAAA` requests to `10.0.0.6`, and all other requests to `10.0.0.1`.

```
. {
  view example {
    expr type() in ['A', 'AAAA']
  }
  forward . 10.0.0.6
}

. {
  forward . 10.0.0.1
}
```

Send all requests for `abc.*.example.com` (where * can be any number of labels), to `10.0.0.2`, and all other
requests to `10.0.0.1`.
Note that the regex pattern is enclosed in single quotes, and backslashes are escaped with backslashes.

```
. {
  view example {
    expr name() matches '^abc\\..*\\.example\\.com\\.$'
  }
  forward . 10.0.0.2
}

. {
  forward . 10.0.0.1
}
```

## Expressions

To evaluate expressions, *view* uses the expr-lang/expr package ( https://github.com/expr-lang/expr ).
For example, an expression could look like:
`(type() == 'A' && name() == 'example.com.') || client_ip() == '1.2.3.4'`.

All expressions should be written to evaluate to a boolean value.

See https://github.com/expr-lang/expr/blob/master/docs/Language-Definition.md as a detailed reference for valid syntax.

### Available Expression Functions

In the context of the *view* plugin, expressions can reference DNS query information by using utility
functions defined below.

#### DNS Query Functions

* `bufsize() int`: the EDNS0 buffer size advertised in the query
* `class() string`: class of the request (IN, CH, ...)
* `client_ip() string`: client's IP address, for IPv6 addresses these are enclosed in brackets: `[::1]`
* `do() bool`: the EDNS0 DO (DNSSEC OK) bit set in the query
* `id() int`: query ID
* `name() string`: name of the request (the domain name requested ending with a dot): `example.com.`
* `opcode() int`: query OPCODE
* `port() string`: client's port
* `proto() string`: protocol used (tcp or udp)
* `server_ip() string`: server's IP address; for IPv6 addresses these are enclosed in brackets: `[::1]`
* `server_port() string` : server's port
* `size() int`: request size in bytes
* `type() string`: type of the request (A, AAAA, TXT, ...)

#### Utility Functions

* `incidr(ip string, cidr string) bool`: returns true if _ip_ is within _cidr_
* `metadata(label string)` - returns the value for the metadata matching _label_

## Metadata

The view plugin will publish the following metadata, if the *metadata*
plugin is also enabled:

* `view/name`: the name of the view handling the current request
