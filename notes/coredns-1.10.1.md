+++
title = "CoreDNS-1.10.1 Release"
description = "CoreDNS-1.10.1 Release Notes."
tags = ["Release", "1.10.1", "Notes"]
release = "1.10.0"
date = "2023-01-20T00:00:00+00:00"
author = "coredns"
+++

This release fixes some bugs, and adds some new features including:
* Corrected architecture labels in multi-arch image manifest
* A new plugin *timeouts* that allows configuration of server listener timeout durations
* *acl* can drop queries as an action
* *template* supports creating responses with extended DNS errors
* New weighted policy in *loadbalance*
* Option to serve original record TTLs from *cache*

## Brought to You By

Arthur Outhenin-Chalandre,
Ben Kaplan,
Chris O'Haver,
Gabor Dozsa,
Grant Spence,
Kumiko as a Service,
LAMRobinson,
Miciah Dashiel Butler Masters,
Ondřej Benkovský,
Rich,
Stephen Kitt,
Yash Singh,
Yong Tang,
rsclarke,
sanyo0714

## Noteworthy Changes

* plugin/timeouts - Allow ability to configure listening server timeouts (https://github.com/coredns/coredns/pull/5784)
* plugin/acl: adding ability to drop queries (https://github.com/coredns/coredns/pull/5722)
* plugin/template : add support for extended DNS errors (https://github.com/coredns/coredns/pull/5659)
* plugin/kubernetes: error NXDOMAIN for TXT lookups (https://github.com/coredns/coredns/pull/5737)
* plugin/kubernetes: dont match external services when endpoint is specified (https://github.com/coredns/coredns/pull/5734)
* plugin/k8s_external: Fix rcode for headless services (https://github.com/coredns/coredns/pull/5657)
* plugin/edns: remove truncating of question section on bad EDNS version (https://github.com/coredns/coredns/pull/5787)
* plugin/dnstap: Fix behavior when multiple dnstap plugins specified (https://github.com/coredns/coredns/pull/5773)
* plugin/cache: cache now uses source query DNSSEC option for upstream refresh (https://github.com/coredns/coredns/pull/5671)
* Workaround for incorrect architecture (https://github.com/coredns/coredns/pull/5691)
* plugin/loadbalance: Add weighted policy (https://github.com/coredns/coredns/pull/5662)
* plugin/cache: Add keepttl option (https://github.com/coredns/coredns/pull/5879)
* plugin/forward: Fix dnstap for forwarded request/response (https://github.com/coredns/coredns/pull/5890)