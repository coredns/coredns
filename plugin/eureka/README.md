# eureka

## Name

*eureka* - enables serving zone data from Netflix Eureka.

## Description

This plugin allows serving zones for applications registered with [Netflix Eureka](https://github.com/Netflix/eureka)

## Syntax

~~~ txt
eureka [ZONE...] {
    base_url BASE_URL
    mode MODE
    fallthrough [ZONES...]
    refresh DURATION
    ttl SECONDS
}
~~~

*   **ZONE** the name of the domain to be accessed.

*   **BASE_URL** the base URL of the Eureka Server. This option is **required**.

*   **MODE** the mode of this plugin. Supports `app` (query by App Name) and `vip` (query by VIP Address). This option is **required**.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **ZONES** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then
    only queries for those zones will be subject to fallthrough.

*   **ZONES** zones it should be authoritative for. If empty, the zones from the configuration
    block.

*   `refresh` can be used to control how long between record retrievals from Eureka Server. This is also used as the TTL for the response.

*   **DURATION** A duration string. Defaults to `1m`. If units are unspecified, seconds are assumed.

*   `ttl` change the DNS TTL of the records generated.

*   **SECONDS** TTL seconds. Defaults to `30`.


## Examples

Enable eureka with `app` mode:

~~~ txt
example.org {
    eureka {
        base_url http://eureka.com
        mode app
    }
}
~~~
With the Corefile above, `test.example.org` will get `A` record for all instance IPs associated with **app name** `test` in Eureka.

Enable eureka with `vip` mode:

~~~ txt
example.org {
    eureka {
        base_url http://eureka.com
        mode vip
    }
}
~~~
With the Corefile above, `test.example.org` will get `A` record of all instance IPs associated with **VIP address** `test` in Eureka.
