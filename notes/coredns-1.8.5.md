+++
title = "CoreDNS-1.8.5 Release"
description = "CoreDNS-1.8.5 Release Notes."
tags = ["Release", "1.8.5", "Notes"]
release = "1.8.5"
date = 2021-09-10T07:00:00+00:00
author = "coredns"
+++

This is a rather big release, we now [share plugins among zones in the same server
block](https://github.com/coredns/coredns/pull/4593), which should save memory. Various bug fixes in
a bunch of plugins and not one, but two new plugins. A *geoip* plugin that can report **where** the
query came from and a *header* plugin that allows you to fiddle with (some of) the header bits in a
DNS message.

With this release, the `coredns_cache_misses_total` metric is deprecated.  It will be removed in a later release.
Users should migrate their promQL  to use `coredns_cache_requests_total - coredns_cache_hits_total`.

## Brought to You By

Ben Kochie,
Chris O'Haver,
Jeongwook Park,
Kohei Yoshida,
Licht Takeuchi,
Manuel Rüger,
Mat Lowery,
mfleader,
Miek Gieben,
Ondřej Benkovský,
Qasim Sarfraz,
rouzier,
Sascha Grunert,
Sven Nebel,
Yong Tang.

## Noteworthy Changes

* core: Add -p for port flag (https://github.com/coredns/coredns/pull/4653)
* core: Fix IPv6 case for CIDR format reverse zones (https://github.com/coredns/coredns/pull/4652)
* core: Share plugins among zones in the same server block (https://github.com/coredns/coredns/pull/4593)
* core: Upstream lookups are done with original EDNS options (https://github.com/coredns/coredns/pull/4826)
* plugin/cache: Unset AD flag when DO is not set for cache miss (https://github.com/coredns/coredns/pull/4736)
* plugin/cache: Update cache metrics and add a total cache request counter to follow Prometheus convention (https://github.com/coredns/coredns/pull/4781)
* plugin/errors: Add configurable log level to errors plugin (https://github.com/coredns/coredns/pull/4718)
* plugin/file: fix wildcard CNAME answer (https://github.com/coredns/coredns/pull/4828)
* plugin/forward: Add proxy address as tag (https://github.com/coredns/coredns/pull/4757)
* plugin/geoip: Create geoip plugin (https://github.com/coredns/coredns/pull/4688)
* plugin/header: Introduce header plugin (https://github.com/coredns/coredns/pull/4752)
* plugin/kubernetes: Add NS+hosts records to xfr response. Add coredns service to test data. (https://github.com/coredns/coredns/pull/4696)
* plugin/kubernetes: Improve namespace usage (https://github.com/coredns/coredns/pull/4767)
* plugins/kubernetes: Switch to klog/v2 (https://github.com/coredns/coredns/pull/4778)
* plugin/kubernetes: Only answer transfer requests for authoritative zones (https://github.com/coredns/coredns/pull/4802)
* plugin/log: Do not log NOERROR in log plugin when response is not available (https://github.com/coredns/coredns/pull/4725)
* plugin/log: Fix closing of codeblock (https://github.com/coredns/coredns/pull/4680)
* plugin/metrics: When no response is written, fallback to status of next plugin in prometheus plugin (https://github.com/coredns/coredns/pull/4727)
* plugin/route53: Fix Route53 plugin cannot retrieve ECS Task Role (https://github.com/coredns/coredns/pull/4669)
* plugin/secondary: Doc updates (https://github.com/coredns/coredns/pull/4686)
* plugin/secondary: Retry initial transfer until successful (https://github.com/coredns/coredns/pull/4663)
* plugin/trace: Fix rcode tag in case of no response (https://github.com/coredns/coredns/pull/4742)
* plugin/trace: Publish trace id as metadata from trace plugin (https://github.com/coredns/coredns/pull/4749)
* plugin/trace: Trace plugin can mark traces with error tag (https://github.com/coredns/coredns/pull/4720)
