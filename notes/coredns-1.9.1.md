+++
title = "CoreDNS-1.9.1 Release"
description = "CoreDNS-1.9.1 Release Notes."
tags = ["Release", "1.9.1", "Notes"]
release = "1.9.1"
date = "2022-03-09T00:00:00+00:00"
author = "coredns"
+++

This is a release with security and bug fixes and some new features added. 1.9.1 is also built
with golang 1.17.8 that addressed several golang 1.17.6 vulnerabilities (CVE-2022-23772,
CVE-2022-23773, CVE-2022-23806).
Note golang 1.17.6 was used to built coredns 1.9.0.

## Brought to You By

Chris O'Haver,
Elijah Andrews,
Rudolf Schönecker,
Yong Tang,
nathannaveen,
xuweiwei

## Noteworthy Changes

* plugin/autopath: Don't panic on empty token (https://github.com/coredns/coredns/pull/5169)
* plugin/cache: Add zones label to cache metrics (https://github.com/coredns/coredns/pull/5124)
* plugin/file: Add TXT test case (https://github.com/coredns/coredns/pull/5079)
* plugin/forward: Don't panic when from-zone cannot be normalized (https://github.com/coredns/coredns/pull/5170)
* plugin/grpc: Fix healthy proxy error case (https://github.com/coredns/coredns/pull/5168)
* plugin/grpc: Don't panic when from-zone cannot be normalized (https://github.com/coredns/coredns/pull/5171)
* plugin/k8s_external: Implement zone transfers (https://github.com/coredns/coredns/pull/4977)
* plugin/k8s_external: Fix external nsAddrs when CoreDNS Service has no External IPs (https://github.com/coredns/coredns/pull/4891)
* plugin/kubernetes: Log api connection failures and server start delay (https://github.com/coredns/coredns/pull/5044)
* plugin/log: Expand `{combined}` and `{common}` in log format (https://github.com/coredns/coredns/pull/5230)
* plugin/metrics: Add metric counting DNS-over-HTTPS responses (https://github.com/coredns/coredns/pull/5130)
* plugin/reload: Change hash from md5 to sha512 (https://github.com/coredns/coredns/pull/5226)
* plugin/secondary: Fix startup transfer failure wrong zone logged (https://github.com/coredns/coredns/pull/5085)
