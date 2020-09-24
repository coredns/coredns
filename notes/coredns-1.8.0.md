+++
title = "CoreDNS-1.8.0 Release"
description = "CoreDNS-1.8.0 Release Notes."
tags = ["Release", "1.8.0", "Notes"]
release = "1.8.0"
date = 2020-09-21T10:00:00+00:00
author = "coredns"
draft = "yes"
+++

The CoreDNS team has released
[CoreDNS-1.8.0](https://github.com/coredns/coredns/releases/tag/v1.8.0).

This release add two backwards incompatible changes.

One, because Caddy is now developing a version 2 and we are using version 1 we've internalized Caddy
into <https://github.com/coredns/caddy>. This means the `caddy` types change and *all* plugins need
to fix the import path from: `github.com/caddyserver/caddy` to `github.com/coredns/caddy` (this can
thankfully be automated).

Next the `transfer` plugin is now made a first class citizen and plugins wanting to perform outgoing
zone transfers can now do so: *file*, *auto*, *secondary* and *kubernetes* are now doing so.
You'll need to change your Corefile from (e.g.):

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

## Noteworthy Changes

* plugin/transfer: Implement notifies for transfer plugin (https://github.com/coredns/coredns/pull/3972) (#4142)
* core: Move caddy v1 in our GitHub org (https://github.com/coredns/coredns/pull/4018)
