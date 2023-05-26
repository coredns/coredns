+++
title = "CoreDNS-1.8.7 Release"
description = "CoreDNS-1.8.7 Release Notes."
tags = ["Release", "1.8.7", "Notes"]
release = "1.8.7"
date = "2021-12-09T00:00:00+00:00"
author = "coredns"
+++

This is a release with bug fixes and some new features added. We now enable HTTP/2 in
gRPC service (https://github.com/coredns/coredns/pull/4842). The shuffling algorithm
in loadbalance plugin has also been improved to have a more consistent
behavior (https://github.com/coredns/coredns/pull/4961). This release will also
log deprecation warnings when wildcard queries are received by kubernetes. The
wildcard functionality will be completely removed from kubernetes plugin in
future releases.


## Brought to You By

Chris O'Haver,
Christian Ang,
Cyb3r Jak3,
Denis Tingaikin,
gomakesix,
Hu Shuai,
Humberto Leal,
jayonlau,
Johnny Bergström,
LiuCongran,
Matt Palmer,
Miek Gieben,
OctoHuman,
Ondřej Benkovský,
Pavol Lieskovský,
Vector,
Wu Shuang,
xuweiwei,
xww,
Yong Tang,
ZhangJian He,
Zou Nengren

## Noteworthy Changes

* core: Support plain HTTP for DoH (https://github.com/coredns/coredns/pull/4997)
* plugin/auto: Fix panic caused by config invalid reload value (https://github.com/coredns/coredns/pull/4986)
* plugin/cache: fix data race (https://github.com/coredns/coredns/pull/4932)
* plugin/file: Fix print tree error (https://github.com/coredns/coredns/pull/4962)
* plugin/file: Fix issue of multiple file plugin have same reload time (https://github.com/coredns/coredns/pull/5020)
* plugin/forward: Use new msg.Id for upstream queries (https://github.com/coredns/coredns/pull/4841)
* plugin/grpc: Enable HTTP/2 in gRPC service (https://github.com/coredns/coredns/pull/4842)
* plugin/k8s_external: Fix SRV queries doesn't work with AWS ELB/NLB (https://github.com/coredns/coredns/pull/4929)
* plugin/kubernetes: Add wildcard warnings (https://github.com/coredns/coredns/pull/5030)
* plugin/loadbalance: More consistent shuffling (https://github.com/coredns/coredns/pull/4961)
* plugin/metrics: Support HTTPS qType in requests count metric label (https://github.com/coredns/coredns/pull/4934)
* plugin/metrics: Expand coredns_dns_responses_total with plugin label (https://github.com/coredns/coredns/pull/4914)
* plugin/route53: Configurable AWS Endpoint (https://github.com/coredns/coredns/pull/4963)
