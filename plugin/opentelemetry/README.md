# opentelemetry

## Name

*opentelemetry* - enables [OpenTelemetry](https://opentelemetry.io/docs/) tracing of DNS requests as they go through the plugin chain

## Description

With *opentelemetry* plugin you enable OpenTelemetry tracing of the DNS request flow through CoreDNS. This plugin cannot be used together with [trace plugin](../trace).

## Syntax

The simple form:

~~~
opentelemetry [ENDPOINT-TYPE] [ENDPOINT]
~~~

* **ENDPOINT-TYPE** is the type of traces exporter. Currently `zipkin` or `otelhttp` (OpenTelemetry HTTP exporter) are supported.
  Default type is `zipkin`.
* **ENDPOINT** is the tracing collector destination.
  * For `zipkin` the **ENDPOINT** is in format`http://HOST:PORT/api/v2/spans`.
  Format `HOST:PORT` is also accepted and will be automatically transformed into the URL format. The default value is `localhost:9411`.
  * For `otelhttp` the format of the **ENDPOINT** has to be `HOST:PORT`. Other formats are not accepted. Default value is `localhost:4318`.


Additional features can be enabled with this syntax:

~~~
opentelemetry [ENDPOINT-TYPE] ENDPOINT {
    service NAME
    max_queue_size SIZE
    batch_timeout DURATION
    export_timeout DURATION
    max_export_batch_size SIZE
    sampling_probability PROBABILITY
    insecure
}
~~~

* `service` allows you to specify the service name as the root span attribute reported to the tracing server. The default value is `coredns`.
* `max_queue_size` is the maximum queue size to buffer spans for delayed processing. If the queue is full the spans are dropped.
  The default value of `max_queue_size` is 2048.
* `batch_timeout` is the maximum duration for constructing a batch. The trace processor forcefully sends available spans when the timeout is reached.
  The default value of `batch_timeout` is 5 seconds.
* `export_timeout` specifies the maximum duration for exporting spans. If the timeout is reached, the export will be canceled.
  The default value of `export_timeout` is 30 seconds.
* `max_export_batch_size` is the maximum number of spans to process in a single batch. If there is more than one batch worth of spans
  then it processes multiple batches one by one without any delay. The default value of `max_export_batch_size` is 512.
* `sampling_probability` is a float number between 0 and 1 that represents the probability of trace sampling, where 1 means that all the traces
  will be sent to the collector and 0 means that no traces will be sent to the collector.
* `insecure` enables exporting traces via an insecure channel (like HTTP)


## Metadata

The opentelemetry plugin will publish the following metadata if the *metadata*
plugin is also enabled:

* `opentelemetry/traceid`: identifier of a trace of the processed DNS request


## Examples

Basic `opentelemetry` config with Zipkin trace exporter:

~~~ corefile
. {
    opentelemetry zipkin localhost:9411
}
~~~

`opentelemetry` config with Zipkin trace exporter in URL format:

~~~ corefile
. {
    opentelemetry zipkin http://localhost:9411/api/v2/spans
}
~~~

Basic `opentelemetry` config with insecure (no TLS) OpenTelemetry trace HTTP exporter:

~~~ corefile
. {
    opentelemetry otelhttp localhost:4318 {
       insecure
    }
}
~~~

`opentelemetry` config with Zipkin trace exporter and advanced features:

~~~ corefile
. {
    opentelemetry zipkin localhost:9411 {
        service coredns
        max_queue_size 10
        batch_timeout 50s
        export_timeout 100s
        max_export_batch_size 20
        sampling_probability 0.75
    }
}
~~~
* The service name reported in the root span will be set to `coredns`.
* There will be a maximum of 10 spans in the queue, every next span will be dropped until there is some space in the queue again.
* After 50 seconds of constructing a batch of spans the batch will be sent to the collector even if the batch is not full.
* In case of unavailability to send the spans to the collector the exporter will be retrying for 100 seconds. After that, the export is canceled.
* There will be a maximum of 20 spans in one batch. After that, a new batch will be constructed.
* In this case not every trace will be processed. There is a 75% possibility of processing the trace.
