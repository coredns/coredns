# log

## Name

*log* - enables query logging to standard output.

## Description

By just using *log* you dump all queries (and parts for the reply) on standard output. Options exist
to tweak the output a little. Note that for busy servers logging will incur a performance hit.

Enabling or disabling the *log* plugin only affects the query logging, any other logging from
CoreDNS will show up regardless.

## Syntax

~~~ txt
log
~~~

With no arguments, a query log entry is written to *stdout* in the common log format for all requests.
Or if you want/need slightly more control:

~~~ txt
log [NAMES...] [FORMAT]
~~~

* `NAMES` is the name list to match in order to be logged
* `FORMAT` is the log format to use (default is Common Log Format), `{common}` is used as a shortcut
  for the Common Log Format. You can also use `{combined}` for a format that adds the query opcode
  `{>opcode}` to the Common Log Format.

You can further specify the classes of responses that get logged:

~~~ txt
log [NAMES...] [FORMAT] {
    class CLASSES...
}
~~~

* `CLASSES` is a space-separated list of classes of responses that should be logged

The classes of responses have the following meaning:

* `success`: successful response
* `denial`: either NXDOMAIN or nodata responses (Name exists, type does not). A nodata response
   sets the return code to NOERROR.
* `error`: SERVFAIL, NOTIMP, REFUSED, etc. Anything that indicates the remote server is not willing to
  resolve the request.
* `all`: the default - nothing is specified. Using of this class means that all messages will be
  logged whatever we mix together with "all".

If no class is specified, it defaults to `all`.

## Log Format

You can specify a custom log format with any placeholder values. Log supports both request and
response placeholders.

The following place holders are supported:

* `{type}`: qtype of the request
* `{name}`: qname of the request
* `{class}`: qclass of the request
* `{proto}`: protocol used (tcp or udp)
* `{remote}`: client's IP address, for IPv6 addresses these are enclosed in brackets: `[::1]`
* `{local}`: server's IP address, for IPv6 addresses these are enclosed in brackets: `[::1]`
* `{size}`: request size in bytes
* `{port}`: client's port
* `{duration}`: response duration
* `{rcode}`: response RCODE
* `{rsize}`: raw (uncompressed), response size (a client may receive a smaller response)
* `{>rflags}`: response flags, each set flag will be displayed, e.g. "aa, tc". This includes the qr
  bit as well
* `{>bufsize}`: the EDNS0 buffer size advertised in the query
* `{>do}`: is the EDNS0 DO (DNSSEC OK) bit set in the query
* `{>id}`: query ID
* `{>opcode}`: query OPCODE
* `{common}`: the default Common Log Format.
* `{combined}`: the Common Log Format with the query opcode.
* `{/LABEL}`: any metadata label is accepted as a place holder if it is enclosed between `{/` and
  `}`, the place holder will be replaced by the corresponding metadata value or the default value
  `-` if label is not defined. See the *metadata* plugin for more information.

The default Common Log Format is:

~~~ txt
`{remote}:{port} - {>id} "{type} {class} {name} {proto} {size} {>do} {>bufsize}" {rcode} {>rflags} {rsize} {duration}`
~~~

In the default text mode, each of these logs is output with `log.Info`, so a typical example looks like this:

~~~ txt
[INFO] [::1]:50759 - 29008 "A IN example.org. udp 41 false 4096" NOERROR qr,rd,ra,ad 68 0.037990251s
~~~

## JSON Output

Start CoreDNS with `-log-format=json` to select JSON output for the entire process.
This is a command-line flag, not a Corefile directive. `-log-format=text` is the default.
The `log` plugin's name and response-class filters work identically in both modes.

Each query produces one JSON record with common fields `time`, `level`, `msg`, and
`plugin` (always `log` for query records), plus these typed fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `client_ip` | string | Client address, without brackets around IPv6 addresses |
| `client_port` | number | Client port |
| `qname` | string | Lowercase, fully qualified query name, in DNS presentation format |
| `qtype`, `qclass` | string | Query type and class, including numeric forms for unknown values |
| `protocol` | string | `udp` or `tcp`, as for `{proto}` |
| `id`, `opcode` | number | Query ID and opcode |
| `request_size` | number | Request size in bytes, as for `{size}` |
| `dnssec_ok` | boolean | Query's DNSSEC OK bit |
| `bufsize` | number | Effective response buffer size, as for `{>bufsize}` |
| `rcode` | string or null | Response RCODE, or null if no DNS response was recorded |
| `response_size` | number | Recorded response size in bytes, as for `{rsize}` |
| `duration_seconds` | number | Elapsed handling time in seconds |

As in text mode, response sizes describe recorded, uncompressed messages, not
necessarily the bytes delivered to the client. Deferred errors (such as SERVFAIL)
use the response CoreDNS will generate after the plugin chain returns. A dropped
request has `rcode: null`; it is not logged as a successful response. Raw `Write`
calls contribute to the size but do not provide a decoded response RCODE.

`FORMAT` still controls `msg`, including custom formats and metadata placeholders.
It does not replace the JSON schema or define new top-level fields. The DNS fields
come directly from the request and response, not from parsing `msg`. For example,
with `log . "{name} {rcode}"`:

```json
{"time":"2026-09-15T08:00:00Z","level":"INFO","msg":"example.org. NOERROR","plugin":"log","client_ip":"127.0.0.1","client_port":40212,"qname":"example.org.","qtype":"A","qclass":"IN","protocol":"udp","id":42,"opcode":0,"request_size":29,"dnssec_ok":false,"bufsize":512,"rcode":"NOERROR","response_size":29,"duration_seconds":0.001}
```

## Additional metadata

The log plugin adds the following metadata to allow for granular differentiation of NOERROR denial vs success messages. These are mapped from `plugin/pkg/response/classify.go` and `plugin/pkg/response/typify.go`.

* `{/log/class}`: success, denial
* `{/log/type}`: NODATA, NXDOMAIN, NOERROR

~~~ corefile
. {
    log . "{proto} Request: {name} {type} {/log/class} {/log/type}"
}
~~~

## Examples

Log all requests to stdout

~~~ corefile
. {
    log
    whoami
}
~~~

Custom log format, for all zones (`.`)

~~~ corefile
. {
    log . "{proto} Request: {name} {type} {>id}"
}
~~~

Only log denials (NXDOMAIN and nodata) for example.org (and below)

~~~ corefile
. {
    log example.org {
        class denial
    }
}
~~~

Log all queries which were not resolved successfully in the Combined Log Format.

~~~ corefile
. {
    log . {combined} {
        class denial error
    }
}
~~~

Log all queries on which we did not get errors

~~~ corefile
. {
    log . {
        class denial success
    }
}
~~~

Also the multiple statements can be OR-ed, for example, we can rewrite the above case as following:

~~~ corefile
. {
    log . {
        class denial
        class success
    }
}
~~~
