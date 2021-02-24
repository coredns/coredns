+++
title = "CoreDNS-1.8.3 Release"
description = "CoreDNS-1.8.3 Release Notes."
tags = ["Release", "1.8.3", "Notes"]
release = "1.8.3"
date = 2021-02-24T07:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.3](https://github.com/coredns/coredns/releases/tag/v1.8.3). This release includes a
bunch of bugfixes and a few enhancements, see below.

In case you're wondering, 1.8.2 didn't properly upload and tag the docker images, hence a quick
followup release with that fixed.

## Brought to You By

Bob,
chantra,
Chris O'Haver,
Frank Riley,
George Shammas,
Johnny Bergström,
Jun Chen,
Lars Ekman,
Manuel Rüger,
Maxime Ginters,
Miek Gieben,
slick-nic,
TimYin.

## Noteworthy Changes

* core: Also clear `do` and `size` (https://github.com/coredns/coredns/pull/4465)
* core: Flag blacklisting not needed anymore (https://github.com/coredns/coredns/pull/4420)
* core: Set http request in writer (https://github.com/coredns/coredns/pull/4445)
* Makefile.release: Replace manifest-tool with docker manifest (https://github.com/coredns/coredns/pull/4421)
* Makefile.release: Fix the Makefile (https://github.com/coredns/coredns/pull/4483)
* plugin/acl: add the ability to filter records (https://github.com/coredns/coredns/pull/4389)
* plugin/dnstap: Fix out of order messages and fix forward perspective. (https://github.com/coredns/coredns/pull/4395)
* plugin/forward Add rcode and rtype to request_duration_seconds metric (https://github.com/coredns/coredns/pull/4391)
* plugin/kubernetes: Corrected detection of K8s minor version (https://github.com/coredns/coredns/pull/4430)
* plugin/kubernetes: make kubeconfig argument 'context' optional (https://github.com/coredns/coredns/pull/4451)
* plugin/rewrite: copy msg before rewriting (https://github.com/coredns/coredns/pull/4443)
* plugin/rewrite: SRV targets and additional names in response (https://github.com/coredns/coredns/pull/4287)
* plugin/sign: track zone file's mtime (https://github.com/coredns/coredns/pull/4431)
* plugin/trace: Use compatible tag name for datadog (https://github.com/coredns/coredns/pull/4408)
* plugin/transfer: only allow outgoing axfr over tcp (https://github.com/coredns/coredns/pull/4452)
