+++
title = "CoreDNS-1.8.1 Release"
description = "CoreDNS-1.8.1 Release Notes."
tags = ["Release", "1.8.1", "Notes"]
release = "1.8.1"
date = 2021-01-20T07:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.1](https://github.com/coredns/coredns/releases/tag/v1.8.1).

This release fixes a bunch of bugs, and adds a (very) simple new plugin
called [local](https://coredns.io/plugins/local/) to answer "local" queries. Bunch of work in
the [kubernetes](https://coredns.io/plugins/kubernetes) plugin to add support for new upstream
feautres.

## Brought to You By

Blake Ryan, Bob, Chotiwat Chawannakul, Chris O'Haver, Guangwen Feng, Gunadhya, Jiang Biao, Johnny
Bergström, luanphantiki, Matt Kulka, mgugger, Miek Gieben, Serge, sschepens, Yong Tang, ZouYu.

## Noteworthy Changes

* plugin/{clouddns,azure,route53}: Use cancelable contexts for cloud provider plugin refreshes (https://github.com/coredns/coredns/pull/4226)
* plugin/azure: Iterate over all RecordSetListResultPage Pages (https://github.com/coredns/coredns/pull/4351)
* core: custom DoH request validation (https://github.com/coredns/coredns/pull/4329)
* pkg/tls: remove InsecureSkipVerify=true flag (https://github.com/coredns/coredns/pull/4265)
* plugin/azure: return FQDN as MNAME in SOA record (https://github.com/coredns/coredns/pull/4286)
* plugin/cache Prevent race from prefetching (https://github.com/coredns/coredns/pull/4368)
* plugin/dnssec: Change hash key input (https://github.com/coredns/coredns/pull/4372)
* plugin/dnstap: remove config struct (https://github.com/coredns/coredns/pull/4258)
* plugin/dnstap: remove custom encoder (https://github.com/coredns/coredns/pull/4242)
* plugin/file: Use NXDOMAIN response if CNAME target is NXDOMAIN (https://github.com/coredns/coredns/pull/4303)
* plugin/file: document wrong behavior in lookup fox Apex (https://github.com/coredns/coredns/pull/4376)
* plugin/file: guard against cname loops (https://github.com/coredns/coredns/pull/4387)
* plugin/forward: respond with REFUSED when max_concurrent is exceeded (https://github.com/coredns/coredns/pull/4326)
* plugin/forward: HC every 0.5 seconds, do not do exponential backoff (https://github.com/coredns/coredns/pull/4371)
* plugin/health: Fix health check endpoint (https://github.com/coredns/coredns/pull/4231)
* plugin/kubernetes: Add support for dual stack ClusterIP Services (https://github.com/coredns/coredns/pull/4339)
* plugin/kubernetes: Fix dns programming duration metric (https://github.com/coredns/coredns/pull/4255)
* plugin/kubernetes: Fix NPE issue (https://github.com/coredns/coredns/pull/4338)
* plugin/kubernetes: Watch EndpointSlices (https://github.com/coredns/coredns/pull/4209)
* plugin/local: add local plugin (https://github.com/coredns/coredns/pull/4262)
* plugin/trace: Fix zipkin json_v2 (https://github.com/coredns/coredns/pull/4180)
* plugin/transfer: Fix go-routine leak (https://github.com/coredns/coredns/pull/4380)
