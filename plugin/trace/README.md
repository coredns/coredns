# trace

## Name

*trace* - enables OpenTracing-based tracing of DNS requests as they go through the plugin chain.

## Description

With *trace* you enable OpenTracing of how a request flows through CoreDNS. Enable the *debug*
plugin to get logs from the trace plugin.

## Syntax

The simplest form is just:

~~~
trace [ENDPOINT-TYPE] [ENDPOINT]
~~~

* **ENDPOINT-TYPE** is the type of tracing destination. Currently only `zipkin` and `datadog` are supported.
  Defaults to `zipkin`.
* **ENDPOINT** is the tracing destination, and defaults to `localhost:9411`. For Zipkin, if
  **ENDPOINT** does not begin with `http`, then it will be transformed to `http://ENDPOINT/api/v1/spans`.

With this form, all queries will be traced.

Additional features can be enabled with this syntax:

~~~
trace [ENDPOINT-TYPE] [ENDPOINT] {
    every AMOUNT
    service NAME
    client_server
    datadog_analytics_rate RATE
}
~~~

* `every` **AMOUNT** will only trace one query of each AMOUNT queries. For example, to trace 1 in every
  100 queries, use AMOUNT of 100. The default is 1.
* `service` **NAME** allows you to specify the service name reported to the tracing server.
  Default is `coredns`.
* `client_server` will enable the `ClientServerSameSpan` OpenTracing feature.
* `datadog_analytics_rate` **RATE** will enable [trace analytics](https://docs.datadoghq.com/tracing/app_analytics) on the traces sent
  from *0* to *1*, *1* being every trace sent will be analyzed. This is a datadog only feature
  (**ENDPOINT-TYPE** needs to be `datadog`)

## Zipkin

You can run Zipkin on a Docker host like this:

```
docker run -d -p 9411:9411 openzipkin/zipkin
```

Note the zipkin provider does not support the v1 API since coredns 1.7.1.

## Examples

Use an alternative Zipkin address:

~~~
trace tracinghost:9253
~~~

or

~~~ corefile
. {
    trace zipkin tracinghost:9253
}
~~~

If for some reason you are using an API reverse proxy or something and need to remap
the standard Zipkin URL you can do something like:

~~~
trace http://tracinghost:9411/zipkin/api/v1/spans
~~~

Using DataDog:

~~~
trace datadog localhost:8126
~~~

Trace one query every 10000 queries, rename the service, and enable same span:

~~~
trace tracinghost:9411 {
	every 10000
	service dnsproxy
	client_server
}
~~~

## See Also

See the *debug* plugin for more information about debug logging.
