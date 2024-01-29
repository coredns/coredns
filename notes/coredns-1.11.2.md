+++
title = "CoreDNS-1.11.2 Release"
description = "CoreDNS-1.11.2 Release Notes."
tags = ["Release", "1.11.2", "Notes"]
release = "1.11.2"
date = "2024-01-26T00:00:00+00:00"
author = "coredns"
+++

This release contains some new features, bug fixes, and package updates.
New features include:
* When the _forward_ plugin receives a malformed upstream response that overflows,
  it will now send an empty response to the client with the truncated (TC) bit set to prompt the client
  to retry over TCP.
* The _rewrite_ plugin can now rewrite response codes.
* The _dnstap_ plugin now supports adding metadata to the dnstap `extra` field.

## Brought to You By

Amila Senadheera,
Ben Kochie,
Benjamin,
Chris O'Haver,
Grant Spence,
John Belamaric,
Keita Kitamura,
Marius Kimmina,
Michael Grosser,
Ondřej Benkovský,
P. Radha Krishna,
Rahil Bhimjiani,
Sri Harsha,
Tom Thorogood,
Willow (GHOST),
Yong Tang,
Yuheng,
Zhizhen He,
guangwu,
journey-c,
pschou

## Noteworthy Changes

* plugin/tls: respect the path specified by root plugin (https://github.com/coredns/coredns/pull/6138)
* plugin/auto: warn when auto is unable to read elements of the directory tree (https://github.com/coredns/coredns/pull/6333)
* plugin/etcd: the etcd client adds the DialKeepAliveTime parameter (https://github.com/coredns/coredns/pull/6351)
* plugin/cache: key cache on Checking Disabled (CD) bit (https://github.com/coredns/coredns/pull/6354)
* plugin/forward: Use the correct root domain name in the forward plugin's health checks (https://github.com/coredns/coredns/pull/6395)
* plugin/forward: Handle UDP responses that overflow with TC bit (https://github.com/coredns/coredns/pull/6277)
* plugin/rewrite: fix multi request concurrency issue in cname rewrite (https://github.com/coredns/coredns/pull/6407)
* plugin/rewrite: add rcode as a rewrite option (https://github.com/coredns/coredns/pull/6204)
* plugin/dnstap: add support for "extra" field in payload (https://github.com/coredns/coredns/pull/6226)
* plugin/cache: fix keepttl parsing (https://github.com/coredns/coredns/pull/6250)
