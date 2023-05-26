+++
title = "CoreDNS-1.7.1 Release"
description = "CoreDNS-1.7.1 Release Notes."
tags = ["Release", "1.7.1", "Notes"]
release = "1.7.1"
date = 2020-09-21T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.7.1](https://github.com/coredns/coredns/releases/tag/v1.7.1).

This is a small, incremental release that adds some polish and fixes a bunch of bugs.

## Brought to You By

Ben Kochie,
Ben Ye,
Chris O'Haver,
Cricket Liu,
Grant Garrett-Grossman,
Hu Shuai,
Li Zhijian,
Maxime Guyot,
Miek Gieben,
milgradesec,
Oleg Atamanenko,
Olivier Lemasle,
Ricardo Katz,
Ruslan Drozhdzh,
Yong Tang,
Zhou Hao,
Zou Nengren.

## Noteworthy Changes

* backend: fix root zone usage (https://github.com/coredns/coredns/pull/4039)
* core: Add timeouts for http server (https://github.com/coredns/coredns/pull/3920)
* pkg/upstream: set edns0 and Do when required (https://github.com/coredns/coredns/pull/4055)
* plugin/cache: cache: default to DNSSEC (https://github.com/coredns/coredns/pull/4085)
* plugin/{clouddns,route53}: fix lingering goroutines after restart (https://github.com/coredns/coredns/pull/4096)
* plugin/debug: Enable debug globally if enabled in any server config (https://github.com/coredns/coredns/pull/4007)
* plugin/{etcd,kubernetes}: fix root zone usage (https://github.com/coredns/coredns/pull/4039)
* plugin/forward: add hit/miss metrics for connection cache (https://github.com/coredns/coredns/pull/4114)
* plugin/forward: fix panic when `expire` is configured as 0s (https://github.com/coredns/coredns/pull/4115)
* plugin/forward: init ClientSessionCache in tls.Config (https://github.com/coredns/coredns/pull/4108)
* plugin/forward: Register HealthcheckBrokenCount (https://github.com/coredns/coredns/pull/4021)
* plugin/grpc: Improve gRPC Plugin when backend is not available (https://github.com/coredns/coredns/pull/3966)
* plugins: Using promauto package to ensure all created metrics are properly registered (https://github.com/coredns/coredns/pull/4025).
* plugin/template: Add client IP data (https://github.com/coredns/coredns/pull/4034)
* plugin/trace: fix struct allignment (https://github.com/coredns/coredns/pull/4112)
* plugin/trace: Only with *debug* active enable debug mode for tracing - removes extra logging (https://github.com/coredns/coredns/pull/4016)
* project: Add DCO requirement in Contributing guidelines (https://github.com/coredns/coredns/pull/4008)
