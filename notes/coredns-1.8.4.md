+++
title = "CoreDNS-1.8.4 Release"
description = "CoreDNS-1.8.4 Release Notes."
tags = ["Release", "1.8.4", "Notes"]
release = "1.8.4"
date = 2021-03-24T07:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.4](https://github.com/coredns/coredns/releases/tag/v1.8.4). This release includes a
bunch of bugfixes and a few enhancements, see below.
TODO

## Brought to You By

Chris O'Haver,
Miek Gieben,
Mohammad Yosefpor,
Paco Xu,
Soumya Ghosh Dastidar.

## Noteworthy Changes

* plugin/metrics: remove RR type (https://github.com/coredns/coredns/pull/4534)
* plugin/health: add logging for local health request (https://github.com/coredns/coredns/pull/4533)
* plugin/bind: Bind by interface name (https://github.com/coredns/coredns/pull/4522)
* plugin/bind: Exclude interface or ip address  (https://github.com/coredns/coredns/pull/4543)
* plugin/forward: Add upstream metadata (https://github.com/coredns/coredns/pull/4521)
* plugin/minimal: Add minimal-responses plugin (https://github.com/coredns/coredns/pull/4417)
* plugin/transfer: reply with refused (https://github.com/coredns/coredns/pull/4510)
* plugin/kubernetes: Exclude unready endpoints from endpointslices (https://github.com/coredns/coredns/pull/4580)
* plugin/kubernetes: do endpoint/slice check in retry loop (https://github.com/coredns/coredns/pull/4492)
