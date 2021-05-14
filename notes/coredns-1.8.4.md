+++
title = "CoreDNS-1.8.4 Release"
description = "CoreDNS-1.8.4 Release Notes."
tags = ["Release", "1.8.4", "Notes"]
release = "1.8.4"
date = 2021-05-14T07:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.4](https://github.com/coredns/coredns/releases/tag/v1.8.4). This release includes a
bunch of bugfixes and a few enhancements, and a new (small) plugin called *minimal*.

## Brought to You By

Chris O'Haver,
cuirunxing-hub,
Frank Riley,
Keith Coleman,
Miek Gieben,
milgradesec,
Mohammad Yosefpor,
ntoofu,
Paco Xu,
Soumya Ghosh Dastidar,
Steve Greene,
Théotime Lévêque,
Uwe Krueger,
wangchenglong01,
Yong Tang,
Yury Tsarev.

## Noteworthy Changes

* core: fix reverse zones expansion (https://github.com/coredns/coredns/pull/4538)
* plugin/bind: Bind by interface name (https://github.com/coredns/coredns/pull/4522)
* plugin/bind: Exclude interface or ip address  (https://github.com/coredns/coredns/pull/4543)
* plugin/dnssec: heck for two days of remaining validity (https://github.com/coredns/coredns/pull/4606)
* plugin/dnssec: interface type correction for `periodicClean` sig validity check (https://github.com/coredns/coredns/pull/4608)
* plugin/dnssec: use entire RRset as key input (https://github.com/coredns/coredns/pull/4537)
* plugin/forward: Add upstream metadata (https://github.com/coredns/coredns/pull/4521)
* plugin/health: add logging for local health request (https://github.com/coredns/coredns/pull/4533)
* plugin/health: add logging for local health request (https://github.com/coredns/coredns/pull/4533)
* plugin/kubernetes: do endpoint/slice check in retry loop (https://github.com/coredns/coredns/pull/4492)
* plugin/kubernetes: Exclude unready endpoints from endpointslices (https://github.com/coredns/coredns/pull/4580)
* plugin/metrics: remove RR type (https://github.com/coredns/coredns/pull/4534)
* plugin/minimal: Add minimal-responses plugin (https://github.com/coredns/coredns/pull/4417)
* plugin/rewrite: streamline the ResponseRule handling. (https://github.com/coredns/coredns/pull/4473)
* plugin/sign:  Revert "plugin/sign: track zone file's mtime (https://github.com/coredns/coredns/pull/4431)"
* plugin/transfer: reply with refused (https://github.com/coredns/coredns/pull/4510)
