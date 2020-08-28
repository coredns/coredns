+++
title = "CoreDNS-1.7.1 Release"
description = "CoreDNS-1.7.1 Release Notes."
tags = ["Release", "1.7.1", "Notes"]
release = "1.7.1"
date = 2020-06-15T10:00:00+00:00
author = "coredns"
draft = true
+++

**UNRELEASED**

The CoreDNS team has released
[CoreDNS-1.7.1](https://github.com/coredns/coredns/releases/tag/v1.7.1).

This is a small, incremental release that adds some polish and fixes a bunch of bugs.

## Brought to You By

Ben Kochie,
Ben Ye,
Chris O'Haver,
Cricket Liu,
Grant Garrett-Grossman,
Li Zhijian,
Maxime Guyot,
Miek Gieben,
milgradesec,
Oleg Atamanenko,
Olivier Lemasle,
Ricardo Katz,
Yong Tang,
Zhou Hao,
Zou Nengren.

## Noteworthy Changes

* core: Add timeouts for http server (https://github.com/coredns/coredns/pull/3920).
* plugin/{etcd,kubernetes}: fix root zone usage (https://github.com/coredns/coredns/pull/4039).
* plugin/forward: Register HealthcheckBrokenCount (https://github.com/coredns/coredns/pull/4021).
* plugin/grpc: Improve gRPC Plugin when backend is not available (https://github.com/coredns/coredns/pull/3966)
* plugin/route53: Fix wildcard records issue in route53 plugin (https://github.com/coredns/coredns/pull/4038).
* plugins: Using promauto package to ensure all created metrics are properly registered (https://github.com/coredns/coredns/pull/4025).
* plugin/template: Add client IP data (https://github.com/coredns/coredns/pull/4034).
* plugin/trace: Only with *debug* active enable debug mode for tracing - removes extra logging (https://github.com/coredns/coredns/pull/4016).
* project: Add DCO requirement in Contributing guidelines (https://github.com/coredns/coredns/pull/4008).
