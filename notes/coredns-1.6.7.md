+++
title = "CoreDNS-1.6.7 Release"
description = "CoreDNS-1.6.7 Release Notes."
tags = ["Release", "1.6.7", "Notes"]
release = "1.6.7"
date = 2020-01-28T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.6.7](https://github.com/coredns/coredns/releases/tag/v1.6.7).

This is a fairly small release that resolves some nits and it adds mips64le to the set of
architectures that we create binaries for. See "Noteworthy Changes" for more detail.

## Brought to You By

Antonio Ojea,
Brad P. Crochet,
Dominic Yin,
DrmagicE,
Erfan Besharat,
Jonathan Nagy,
Kohei Yoshida,
Miek Gieben,
Yong Tang,
Zheng Xie,
Zou Nengren.

## Noteworthy Changes

* Add mips64le to released architectures (https://github.com/coredns/coredns/pull/3589)
* Fix HostPortOrFile to support IPv6 addresses with zone (https://github.com/coredns/coredns/pull/3527)
* plugin/acl: Document metrics in README (https://github.com/coredns/coredns/pull/3605)
* plugin/cache: Registry cache_miss logic (https://github.com/coredns/coredns/pull/3578)
* plugin/cache: Update comment to conform to the implementation (https://github.com/coredns/coredns/pull/3573)
* plugin/{forward, grpc}: Dedup policy implement between grpc and proxy plugin (https://github.com/coredns/coredns/pull/3537)
* plugin/kubernetes: Bump kubernetes plugin schema version (https://github.com/coredns/coredns/pull/3554)
* plugin/{kubernetes, etc}: Resolve TXT records via CNAME (https://github.com/coredns/coredns/pull/3557)
* plugin/logs: Docs: update README and log plugin (https://github.com/coredns/coredns/pull/3602)
* plugin/sign: Add expiration jitter (https://github.com/coredns/coredns/pull/3588)
