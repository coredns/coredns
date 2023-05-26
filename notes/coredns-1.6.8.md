+++
title = "CoreDNS-1.6.8 Release"
description = "CoreDNS-1.6.8 Release Notes."
tags = ["Release", "1.6.8", "Notes"]
release = "1.6.8"
date = 2020-03-24T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.6.8](https://github.com/coredns/coredns/releases/tag/v1.6.8).

Again a small release with some nice improvements in the *forward* plugin, and overall polish. See
"Noteworthy Changes" for more detail.

Note that 1.7.0 will contain a bunch of backward incompatible changes: the *federation* plugin will
be full removed and the metrics name will be changed to inline with the naming recommendation from
the Prometheus project.

## Brought to You By

Andy Bursavich,
Chris O'Haver,
Christian Tryti,
Darshan Chaudhary,
Kohei Yoshida,
LongKB,
Miek Gieben,
Ricky S,
Sylvain Rabot,
Zou Nengren.

## Noteworthy Changes

* plugin/azure: Add private DNS support for azure plugin (https://github.com/coredns/coredns/pull/1516)
* plugin/cache: explain drop metric (https://github.com/coredns/coredns/pull/3706)
* plugin/forward: Add configuration flag to set if RecursionDesired should be set on health checks (https://github.com/coredns/coredns/pull/3679)
* plugin/forward: Add exponential backoff to healthcheck (https://github.com/coredns/coredns/pull/3643)
* plugin/forward: Add max_concurrent option (https://github.com/coredns/coredns/pull/3640)
* plugin/hosts: Modifies NODATA handling (https://github.com/coredns/coredns/pull/3536)
* plugin/kubernetes: fix metadata (https://github.com/coredns/coredns/pull/3642)
* plugin/kubernetes: Return all records with matching IP for reverse queries (https://github.com/coredns/coredns/pull/3687)
* plugin/metrics: Add query type to latency as well (https://github.com/coredns/coredns/pull/3685)
* plugin/pkg/up: Make default intervals shorter (https://github.com/coredns/coredns/pull/3651)
