# trace

This module enables OpenTracing-based tracing of DNS requests as they go through the
middleware chain.

## Syntax

~~~
trace [endpoint-type] [endpoint]
~~~

For each server you which to trace.

It optionally the endpoint type and endpoint. The type defaults to Zipkin and the
endpoint to localhost:9411. A single argument will be interpreted as a Zipkin endpoint.

The only endpoint type supported so far is Zipkin. You can run Zipkin on a Docker host
like this:

```
docker run -d -p 9411:9411 openzipkin/zipkin
```

For Zipkin, if the endpoint does not begin with `http`, then it will be transformed to
`http://$endpoint/api/v1/spans`.

## Examples

Use an alternative Zipkin address:

~~~
trace tracinghost:9253
~~~

or

~~~
trace zipkin tracinghost:9253
~~~

If for some reason you are using an API reverse proxy or something and need to remap
the standard Zipkin URL you can do something like:

~~~
trace http://tracinghost:9411/zipkin/api/v1/spans
~~~
