+++
title = "CoreDNS-1.9.2 Release"
description = "CoreDNS-1.9.2 Release Notes."
tags = ["Release", "1.9.2", "Notes"]
release = "1.9.2"
date = "2022-05-13T00:00:00+00:00"
author = "coredns"
+++

This is a release with many added features and security and bug fixes. The most notable one is the
release of 3rd party security audit from Trail of Bits. Security issues discovered by this audit
have all been fixed or covered.

## Brought to You By

Antoine Tollenaere,
Balazs Nagy,
Chris O'Haver,
dilyevsky,
hansedong,
Lorenz Brun,
Marius Kimmina,
nathannaveen,
Ondřej Benkovský,
Patrick W. Healy,
Qasim Sarfraz,
xuweiwei,
Yong Tang

## Noteworthy Changes

* core: add Trail of Bits to list of 3rd party security auditors (https://github.com/coredns/coredns/pull/5356)
* core: avoid usage of pseudo-random number (https://github.com/coredns/coredns/pull/5228)
* plugin/bufsize: don't add OPT RR to non-EDNS0 queries (https://github.com/coredns/coredns/pull/5368)
* plugin/cache: add refresh mode setting to serve_stale (https://github.com/coredns/coredns/pull/5131)
* plugin/cache: fix cache poisoning exploit (https://github.com/coredns/coredns/pull/5174)
* plugin/etcd: fix multi record TXT lookups (https://github.com/coredns/coredns/pull/5293)
* plugin/forward: configurable domain support for healthcheck (https://github.com/coredns/coredns/pull/5281)
* plugin/geoip: read source IP from EDNS0 subnet if provided (https://github.com/coredns/coredns/pull/5183)
* plugin/health: rework overloaded goroutine to support graceful shutdown (https://github.com/coredns/coredns/pull/5244)
* plugin/k8s_external: persist tc bit from lookup to client response (https://github.com/coredns/coredns/pull/4716)
* plugin/k8s_external: set authoritative bit in responses (https://github.com/coredns/coredns/pull/5284)
* plugin/kubernetes: fix k8s start up timeout ticker (https://github.com/coredns/coredns/pull/5361)
* plugin/route53: deprecate plaintext secret in Corefile for route53 plugin (https://github.com/coredns/coredns/pull/5228)
* plugin/route53: expand AWS config/credentials setup. (https://github.com/coredns/coredns/pull/5370)
* plugin/template: fix rcode option documentation (https://github.com/coredns/coredns/pull/5328)
