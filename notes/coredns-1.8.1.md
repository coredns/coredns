+++
title = "CoreDNS-1.8.1 Release"
description = "CoreDNS-1.8.1 Release Notes."
tags = ["Release", "1.8.1", "Notes"]
release = "1.8.1"
date = 2020-12-22T08:00:00+00:00
author = "coredns"
draft = true
+++

DRAFT DRAFT DRAFT

The CoreDNS team has released
[CoreDNS-1.8.1](https://github.com/coredns/coredns/releases/tag/v1.8.1).

## Brought to You By

Blake Ryan, Bob, Chotiwat Chawannakul, Chris O'Haver, Guangwen Feng, Jiang Biao, Johnny Bergström, Matt Kulka, Miek Gieben, Serge, Yong Tang, ZouYu, mgugger

## Noteworthy Changes

* plugin/{clouddns,azure,route53}: Use cancelable contexts for cloud provider plugin refreshes (https://github.com/coredns/coredns/pull/4226)
* plugin/health: Fix health check endpoint (https://github.com/coredns/coredns/pull/4231)
* plugin/local: add local plugin (https://github.com/coredns/coredns/pull/4262)
* plugin/kubernetes: Watch EndpointSlices (https://github.com/coredns/coredns/pull/4209)
* plugin/dnstap: remove custom encoder (https://github.com/coredns/coredns/pull/4242)
* plugin/dnstap: remove config struct (https://github.com/coredns/coredns/pull/4258)
* plugin/local: add local plugin (https://github.com/coredns/coredns/pull/4262)
* pkg/tls: remove InsecureSkipVerify=true flag (https://github.com/coredns/coredns/pull/4265)
* plugin/trace: Fix zipkin json_v2 (https://github.com/coredns/coredns/pull/4180)
* plugin/azure: return FQDN as MNAME in SOA record (https://github.com/coredns/coredns/pull/4286)
* plugin/kubernetes: Fix dns programming duration metric (https://github.com/coredns/coredns/pull/4255)
* plugin/file: Use NXDOMAIN response if CNAME target is NXDOMAIN (https://github.com/coredns/coredns/pull/4303)
* plugin/cache: Move .LocalAddr() out of goroutine (https://github.com/coredns/coredns/pull/4281)
* plugin/forward: respond with REFUSED when max_concurrent is exceeded (https://github.com/coredns/coredns/pull/4326)
* core: custom DoH request validation (https://github.com/coredns/coredns/pull/4329)
* plugin/kubernetes: Fix NPE issue (https://github.com/coredns/coredns/pull/4338)
* plugin/kubernetes: Add support for dual stack ClusterIP Services (https://github.com/coredns/coredns/pull/4339)
* plugin/azure: Iterate over all RecordSetListResultPage Pages (https://github.com/coredns/coredns/pull/4351)
