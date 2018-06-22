# metadata

## Name

*metadata* - enable  a metadata collector.

## Description

By enabling *metadata* any plugin that implements [metadata.metadater interface](https://godoc.org/github.com/coredns/coredns/plugin/metadata#Metadater) will be called and will be able to add it's own Metadata to context. The metadata collected will be available for all plugins handler, via the Context parameter provided in the ServeDNS function

## Syntax

~~~
metadata [ZONES... ]
~~~

## Plugins

Any plugin that implements the Metadater interface will be called to get metadata information.
If **ZONES** is used it specifies all the zones the plugin should add Metadata to.
Metadata is added to all context going through metadater if **ZONES** are not specified.

## Examples

Enable metadata

~~~ corefile
. {
    metadata
}
~~~

Add metadata for all requests within `example.org.`.

~~~ corefile
. {
    metadata example.org
}
~~~
