# dnstap

## Name

*dnstap* - enables logging to dnstap.

## Description

dnstap is a flexible, structured binary log format for DNS software; see https://dnstap.info. With this
plugin you make CoreDNS output dnstap logging.

Every message is sent to the socket as soon as it comes in, the *dnstap* plugin has a buffer of
10000 messages, above that number dnstap messages will be dropped (this is logged).

## Syntax

~~~ txt
dnstap SOCKET [full] [writebuffer] [queue] {
  [identity IDENTITY]
  [version VERSION]
  [extra EXTRA]
  [skipverify]
}
~~~

* **SOCKET** is the socket (path) supplied to the dnstap command line tool.
* `full` to include the wire-format DNS message.
* **IDENTITY** to override the identity of the server. Defaults to the hostname.
* **VERSION** to override the version field. Defaults to the CoreDNS version.
* **EXTRA** to define "extra" field in dnstap payload, [metadata](../metadata/) replacement available here.
* `skipverify` to skip tls verification during connection. Default to be secure

## Examples

Log information about client requests and responses to */tmp/dnstap.sock*.

~~~ txt
dnstap /tmp/dnstap.sock
~~~

Log information about client requests and responses and tcp write buffer is 1024*Mb and queue is 2048*10000. 

~~~ txt
dnstap /tmp/dnstap.sock full 1024 2048
~~~

Log information including the wire-format DNS message about client requests and responses to */tmp/dnstap.sock*.

~~~ txt
dnstap unix:///tmp/dnstap.sock full
~~~

Log to a remote endpoint.

~~~ txt
dnstap tcp://127.0.0.1:6000 full
~~~

Log to a remote endpoint by FQDN.

~~~ txt
dnstap tcp://example.com:6000 full
~~~

Log to a socket, overriding the default identity and version.

~~~ txt
dnstap /tmp/dnstap.sock {
  identity my-dns-server1
  version MyDNSServer-1.2.3
}
~~~

Log to a socket, customize the "extra" field in dnstap payload. You may use metadata provided by other plugins in the extra field.

~~~ txt
forward . 8.8.8.8
metadata
dnstap /tmp/dnstap.sock {
  extra "upstream: {/forward/upstream}"
}
~~~

Log to a remote TLS endpoint.

~~~ txt
dnstap tls://127.0.0.1:6000 full {
  skipverify
}
~~~

You can use _dnstap_ more than once to define multiple taps. The following logs information including the
wire-format DNS message about client requests and responses to */tmp/dnstap.sock*,
and also sends client requests and responses without wire-format DNS messages to a remote FQDN.

~~~ txt
dnstap /tmp/dnstap.sock full
dnstap tcp://example.com:6000
~~~

## Command Line Tool

Dnstap has a command line tool that can be used to inspect the logging. The tool can be found
at GitHub: <https://github.com/dnstap/golang-dnstap>. It's written in Go.

The following command listens on the given socket and decodes messages to stdout.

~~~ sh
$ dnstap -u /tmp/dnstap.sock
~~~

The following command listens on the given socket and saves message payloads to a binary dnstap-format log file.

~~~ sh
$ dnstap -u /tmp/dnstap.sock -w /tmp/test.dnstap
~~~

Listen for dnstap messages on port 6000.

~~~ sh
$ dnstap -l 127.0.0.1:6000
~~~

## Using Dnstap in your plugin

In your setup function, collect and store a list of all *dnstap* plugins loaded in the config:

~~~ go
x :=  &ExamplePlugin{}

c.OnStartup(func() error {
    if taph := dnsserver.GetConfig(c).Handler("dnstap"); taph != nil {
        for tapPlugin, ok := taph.(*dnstap.Dnstap); ok; tapPlugin, ok = tapPlugin.Next.(*dnstap.Dnstap) {
            x.tapPlugins = append(x.tapPlugins, tapPlugin)
        }
    }
    return nil
})
~~~

And then in your plugin:

~~~ go
import (
  "github.com/coredns/coredns/plugin/dnstap/msg"
  "github.com/coredns/coredns/request"

  tap "github.com/dnstap/golang-dnstap"
)

func (x ExamplePlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
    for _, tapPlugin := range x.tapPlugins {
        q := new(msg.Msg)
        msg.SetQueryTime(q, time.Now())
        msg.SetQueryAddress(q, w.RemoteAddr())
        if tapPlugin.IncludeRawMessage {
            buf, _ := r.Pack() // r has been seen packed/unpacked before, this should not fail
            q.QueryMessage = buf
        }
        msg.SetType(q, tap.Message_CLIENT_QUERY)
        
        // if no metadata interpretation is needed, just send the message
        tapPlugin.TapMessage(q)

        // OR: to interpret the metadata in "extra" field, give more context info
        tapPlugin.TapMessageWithMetadata(ctx, q, request.Request{W: w, Req: query})
    }
    // ...
}
~~~

## See Also

The website [dnstap.info](https://dnstap.info) has info on the dnstap protocol. The *forward*
plugin's `dnstap.go` uses dnstap to tap messages sent to an upstream.
