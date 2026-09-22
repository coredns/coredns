+++
title = "CoreDNS-1.12.2 Release"
description = "CoreDNS-1.12.2 Release Notes."
tags = ["Release", "1.12.2", "Notes"]
release = "1.12.2"
date = "2024-05-30T00:00:00+00:00"
author = "coredns"
+++

This release introduces significant improvements to plugin stability and extensibility.
It adds multicluster support to the Kubernetes plugin, fallthrough support in the file plugin,
and a new SetProxyOptions function for the forward plugin.
Notably, the QUIC (DoQ) plugin now limits concurrent streams, improving performance under load.
Several bug fixes and optimizations improve reliability across plugins, including rewrite, proxy, and metrics.

## Brought to You By

Ambrose Chua,
Arthur Outhenin-Chalandre,
Ben Kochie,
Colden Cullen,
Gleb Kogtev,
Hirotaka Tagawa,
Kevin Lyda,
Manuel Rüger,
Mark Mickan,
Parfenov Ivan,
skipper,
vdbe,
Viktor Oreshkin,
Ville Vesilehto,
Yannick Epstein,
Yong Tang


## Noteworthy Changes

* core: Enable plugins via environment during build (https://github.com/coredns/coredns/pull/7310)
* core: Ensure DNS query name reset in plugin.NS error path (https://github.com/coredns/coredns/pull/7142)
* plugin/forward: Added SetProxyOptions function for forward plugin (https://github.com/coredns/coredns/pull/7229)
* plugin/ready: Do not interrupt querying readiness probes for plugins (https://github.com/coredns/coredns/pull/6975)
* plugin/secondary: Make transfer property mandatory (https://github.com/coredns/coredns/pull/7249)
* plugin/rewrite: Truncated upstream response (https://github.com/coredns/coredns/pull/7277)
* plugin/quic: Limit concurrent DoQ streams and goroutines (https://github.com/coredns/coredns/pull/7296)
* plugin/kubernetes: Add multicluster support (https://github.com/coredns/coredns/pull/7266)
* plugin/bind: Remove zone for link-local IPv4 (https://github.com/coredns/coredns/pull/7295)
* plugin/metrics: Preserve request size from plugins (https://github.com/coredns/coredns/pull/7313)
* plugin/proxy: Avoid Dial hang after Transport stopped (https://github.com/coredns/coredns/pull/7321)
* plugin/file: Add fallthrough support (https://github.com/coredns/coredns/pull/7327)
* plugin/kubernetes: Optimize AutoPath slice allocation (https://github.com/coredns/coredns/pull/7323)
