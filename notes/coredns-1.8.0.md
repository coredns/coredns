+++
title = "CoreDNS-1.8.0 Release"
description = "CoreDNS-1.8.0 Release Notes."
tags = ["Release", "1.8.0", "Notes"]
release = "1.8.0"
date = 2020-10-22T08:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.0](https://github.com/coredns/coredns/releases/tag/v1.8.0).

If you are running 1.7.1 you want to upgrade for the *cache* plugin fixes.

This release also adds three backwards incompatible changes. This will only affect you if you have an
**external plugin** or use **outgoing zone transfers**. If you're using `dnstap` in your plugin,
you'll need to upgrade to the new API as detailed in it's [documentation](/plugins/dnstap).

Two, because Caddy is now developing a version 2 and we are using version 1, we've internalized
Caddy into <https://github.com/coredns/caddy>. This means the `caddy` types change and *all* plugins
need to fix the import path from: `github.com/caddyserver/caddy` to `github.com/coredns/caddy` (this
can thankfully be automated).

And lastly, the `transfer` plugin is now made a first class citizen and plugins wanting to perform
outgoing zone transfers now use this plugin: *file*, *auto*, *secondary* and *kubernetes* are
converted. For this you must change your Corefile from (e.g.):

``` txt
example.org {
    file example.org.signed {
        transfer to *
        transfer to 10.240.1.1
    }
}
```

To

``` txt
example.org {
    file example.org.signed
    transfer {
        to * 10.240.1.1
    }
}
```

## Brought to You By

Bob,
Chris O'Haver,
Johnny Bergström,
Macks,
Miek Gieben,
Yong Tang.

## Noteworthy Changes
* core: doh support: fix alpn for http/2 upgrade when using DoH (https://github.com/coredns/coredns/pull/4182)
* core: doh support: make no TLS config fatal (https://github.com/coredns/coredns/pull/4162)
* core: fix crash with no plugins (https://github.com/coredns/coredns/pull/4184)
* core: Move caddy v1 in our GitHub org (https://github.com/coredns/coredns/pull/4018)
* plugin/auto: allow fallthrough if no zones match (https://github.com/coredns/coredns/pull/4166)
* plugin/cache: Fix filtering (https://github.com/coredns/coredns/pull/4148)
* plugin/cache: Fix removing OPT (https://github.com/coredns/coredns/pull/4190)
* plugin/dnstap: various cleanups (https://github.com/coredns/coredns/pull/4179)
* plugin/ready: don't return 200 during shutdown (https://github.com/coredns/coredns/pull/4167)
* plugin/trace: root span names no longer contain the query data (https://github.com/coredns/coredns/pull/4171)
* plugin/transfer: Implement notifies for transfer plugin (https://github.com/coredns/coredns/pull/3972)
