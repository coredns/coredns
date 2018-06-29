# resolve

## Name

*resolve* - performs `CNAME` and `SRV` target resolution by adding corresponding records for each `CNAME`/`SRV` record in a response.


## Description

For each `CNAME` in a response, the plugin will perform a self-lookup of that `CNAME` target and add the result to the response `ANSWER` section.
`CNAME` chains are supported, i.e. if a `CNAME` points to another `CNAME`, all members of the chain will be added to the result including the terminating (non-CNAME) record.
If a `CNAME` chain loop is encountered (no terminating record) all members of the chain are added to the result (without duplicates).

Similarly, for each `SRV` in a response, the plugin will perform a self-lookup of that `SRV` target and add the result to the response `ADDITIONAL` section.


This plugin can only be used once per Server Block.

## Syntax

~~~ txt
resolve [ZONES...] {
    [no] cname
    [no] srv
}
~~~

* **ZONES**: the zones it should perform `CNAME` target lookups for. These zones refer to the question name zone, not CNAME target zones.
If empty, the zones from the configuration block are used.
* **cname**: enables `CNAME` target resolution (default).  `no cname` disables `CNAME` target resolution.
* **srv**: enables `SRV` target resolution (default).  `no srv` disables `SRV` target resolution.

## Examples

Enable `CNAME` and `SRV` target resolution for all zones.

~~~ corefile
. {
    resolve .
}
~~~

Enable `CNAME` target resolution for all zones, but disable `SRV` target resolution.

~~~ corefile
. {
    resolve . {
        no srv
    }
}
~~~
