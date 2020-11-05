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
dnstap SOCKET [full]
~~~

* **SOCKET** is the socket (path) supplied to the dnstap command line tool.
* `full` to include the wire-format DNS message.

## Examples

Log information about client requests and responses to */tmp/dnstap.sock*.

~~~ txt
dnstap /tmp/dnstap.sock
~~~

Log information including the wire-format DNS message about client requests and responses to */tmp/dnstap.sock*.

~~~ txt
dnstap unix:///tmp/dnstap.sock full
~~~

Log to a remote endpoint.

~~~ txt
dnstap tcp://127.0.0.1:6000 full
~~~

## Command Line Tool

Dnstap has a command line tool that can be used to inspect the logging. The tool can be found
at Github: <https://github.com/dnstap/golang-dnstap>. It's written in Go.

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

In your setup function, check to see if the *dnstap* plugin is loaded:

~~~ go
c.OnStartup(func() error {
    if taph := dnsserver.GetConfig(c).Handler("dnstap"); taph != nil {
        if tapPlugin, ok := taph.(dnstap.Dnstap); ok {
            f.tapPlugin = &tapPlugin
        }
    }
    return nil
})
~~~

And then in your plugin:

~~~ go
func (x RandomPlugin) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {
    if tapPlugin != nil {
        q := new(msg.Msg)
        msg.SetQueryTime(q, time.Now())
        msg.SetQueryAddress(q, w.RemoteAddr())
        if tapPlugin.IncludeRawMessage {
            buf, _ := r.Pack() // r has been seen packed/unpacked before, this should not fail
            q.QueryMessage = buf
        }
        msg.SetType(q, tap.Message_CLIENT_QUERY)
        tapPlugin.TapMessage(q)
    }
    // ...
}
~~~

## See Also

The website [dnstap.info](https://dnstap.info) has info on the dnstap protocol. The *forward*
plugin's `dnstap.go` uses dnstap to tap messages sent to an upstream.
