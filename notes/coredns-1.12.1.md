+++
title = "CoreDNS-1.12.1 Release"
description = "CoreDNS-1.12.1 Release Notes."
tags = ["Release", "1.12.1", "Notes"]
release = "1.12.1"
date = "2024-03-24T00:00:00+00:00"
author = "coredns"
+++

In this release:
* kubernetes: Revert recent change to only create PTR records for endpoints with hostname defined.
* forward: added option to return SERVFAIL immediately if all upstreams are unhealthy.

## Brought to You By

Adrian Moisey,
Arthur Outhenin-Chalandre,
Bartosz Borkowski,
Ben Kochie,
Chris O'Haver,
Min Woo Kim,
Puneet Loya,
Rich,
Viktor,
momantech


## Noteworthy Changes

* core: Increase CNAME lookup limit from 7 to 10 (https://github.com/coredns/coredns/pull/7153)
* plugin/kubernetes: Fix handling of pods having DeletionTimestamp set (https://github.com/coredns/coredns/pull/7119) (#7131)
* plugin/kubernetes: Revert "only create PTR records for endpoints with hostname defined (https://github.com/coredns/coredns/pull/6898)" (#7194)
* plugin/forward: added option `failfast_all_unhealthy_upstreams` to return servfail if all upstreams are down (https://github.com/coredns/coredns/pull/6999)
