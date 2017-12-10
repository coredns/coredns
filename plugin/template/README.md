# template

*template* allows for dynamic responses base on the incoming query.

## Syntax

~~~
template CLASS TYPE [REGEX...] {
    [answer section]
    [answer section]
    [additional section]
    [...]
    [rcode  responsecode]
}
~~~

* **CLASS** the query class (usually IN or ANY)
* **TYPE** the query type (A, PTR, ...)
* **REGEX** regex that are matched against the incoming query. Specifying no regex matches everything.
* `section` A RFC 1035 style fragment build by a go template that contains the answer.
* `resplonsecode` A response code (NXDOMAIN, SERVFAIL, ...)

At least one answer section or rcode is needed.

## Templates

Each answer section is a full-featured go templated with the following predefined data
* `.Name` the query name, as a string
* `.Class` the query class (usually `IN`)
* `.Type` the RR type requested (e.g. `PTR`)
* `.Match` an array of all matches. `index .Match 0` refers to the whole match.
* `.Group` a map fo the named capture groups.
* `.Message` the incoming DNS query message.
* `.Question` the matched question section.

The output of the template must be a RFC 1035 style RR line (commonly refered to as a "zone file").

**WARNING** there is a syntactical problem with go templates and caddy config files. Expressions like `{{$var}}` will be interpreted as a reference to an envorionment variable while `{{ $var }}` will work. Try to avoid template variables.

## Examples

### Resolve .invalid as NXDOMAIN

The `.invalid` domain is a reserved TLD to indicate non-existing domains.

~~~ corefile
. {
    proxy . 8.8.8.8

    template ANY ANY "[.]invalid[.]$" {
      rcode NXDOMAIN
      answer "invalid. 60 {{ .Class }} SOA a.invalid. b.invalid. (1 60 60 60 60)"
    }
}
~~~

1. A query to .invalid will result in NXDOMAIN (rcode)
2. A dummy SOA record is send to hand out a TTL of 60s for caching
3. Querying `.invalid` of `CH` will also cause a NXDOMAIN/SOA response

### Block invalid search domain completions

Imagine you run `example.com` with a datacenter `dc1.example.com`. The datacenter domain
is part of the DNS search domain.
However `something.example.com.dc1.example.com` would usually indicate a wrong autocomplete.

~~~ corefile
. {
    proxy . 8.8.8.8

    template IN ANY "[.](example[.]com[.]dc1[.]example[.]com[.])$" {
      rcode NXDOMAIN
      answer "{{ index .Match 1 }} 60 IN SOA a.{{ index .Match 1 }} b.{{ index .Match 1 }} (1 60 60 60 60)"
    }
}
~~~

1. Using numbered matches works well if there are very few groups (1-4)

### Resolve A/PTR for .example

~~~ corefile
. {
    proxy . 8.8.8.8

    # ip-a-b-c-d.example.com A a.b.c.d

    template IN A (^|[.])ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$ {
      answer "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
    }

    # d.c.b.a.in-addr.arpa PTR ip-a-b-c-d.example

    template IN PTR ^(?P<d>[0-9]*)[.](?P<c>[0-9]*)[.](?P<b>[0-9]*)[.]10[.]in-addr[.]arpa[.]$ {
      answer "{{ .Name }} 60 IN PTR ip-10-{{ .Group.b }}-{{ .Group.c }}-{{ .Group.d }}.example.com."
    }
}
~~~

An IP consists of 4 bytes, `a.b.c.d`. Named groups make it less error prone to reverse the
ip in the PTR case. Try to use named groups to explain what your regex and template are doing.

Note that the A record is actually a wildcard, any subdomain of the ip will resolve to the ip.

Having templates to map certain PTR/A pairs is a common pattern.

### Resolve multiple ip patterns

~~~ corefile
. {
    proxy 8.8.8.8

    template IN A "^ip-(?P<a>10)-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]dc[.]example[.]$" "^(?P<a>[0-9]*)[.](?P<b>[0-9]*)[.](?P<c>[0-9]*)[.](?P<d>[0-9]*)[.]ext[.]example[.]$" {
      answer "{{ .Name }} 60 IN A {{ .Group.a}}.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
    }
}
~~~

Named capture groups can be used to template one response for multiple patterns.

### Resolve A and MX records for ip templates in .example

~~~ corefile
. { 
    proxy . 8.8.8.8
    
    template IN A ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$ {
      answer "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
    }
    template IN MX ^ip-10-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$ {
      answer "{{ .Name }} 60 IN MX 10 {{ .Name }}"
      additional "{{ .Name }} 60 IN A 10.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
    }
}
~~~
