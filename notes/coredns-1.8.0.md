+++
title = "CoreDNS-1.8.0 Release"
description = "CoreDNS-1.8.0 Release Notes."
tags = ["Release", "1.8.0", "Notes"]
release = "1.8.0"
date = 2020-10-01T10:00:00+00:00
author = "coredns"
draft = true
+++

**NOT RELEASED**

The CoreDNS team has released
[CoreDNS-1.8.0](https://github.com/coredns/coredns/releases/tag/v1.8.0).

If you are running 1.7.1 you probably want to upgrade for the *cache* plugin fix.

This release also adds two backwards incompatible changes.

One, because Caddy is now developing a version 2 and we are using version 1, we've internalized Caddy
into <https://github.com/coredns/caddy>. This means the `caddy` types change and *all* plugins need
to fix the import path from: `github.com/caddyserver/caddy` to `github.com/coredns/caddy` (this can
thankfully be automated).

Next the `transfer` plugin is now made a first class citizen and plugins wanting to perform outgoing
zone transfers now use this plugin: *file*, *auto*, *secondary* and *kubernetes* are converted.
For this you must change your Corefile from (e.g.):

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

Miek Gieben,
Yong Tang.

## Noteworthy Changes

* plugin/cache: Fix filtering (https://github.com/coredns/coredns/pull/4148)
* plugin/transfer: Implement notifies for transfer plugin (https://github.com/coredns/coredns/pull/3972) (#4142)
* core: Move caddy v1 in our GitHub org (https://github.com/coredns/coredns/pull/4018)
