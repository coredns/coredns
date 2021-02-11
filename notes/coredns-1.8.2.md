+++
title = "CoreDNS-1.8.2 Release"
description = "CoreDNS-1.8.2 Release Notes."
tags = ["Release", "1.8.2", "Notes"]
release = "1.8.2"
date = 2021-01-20T07:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.8.2](https://github.com/coredns/coredns/releases/tag/v1.8.2).

## Brought to You By

Bob,
Chris O'Haver,
Frank Riley,
George Shammas,
Jun Chen,
Lars Ekman,
Manuel Rüger,
Maxime Ginters,
Miek Gieben,
TimYin.

## Noteworthy Changes

* core: flag blacklisting not needed anymore (https://github.com/coredns/coredns/pull/4420)
* Corrected detection of K8s minor version (https://github.com/coredns/coredns/pull/4430)
* Makefile.release: Replace manifest-tool with docker manifest (https://github.com/coredns/coredns/pull/4421)
* plugin/acl: add the ability to filter records (https://github.com/coredns/coredns/pull/4389)
* plugin/dnstap: Fix out of order messages and fix forward perspective. (https://github.com/coredns/coredns/pull/4395)
* plugin/forward Add rcode and rtype to request_duration_seconds metric (https://github.com/coredns/coredns/pull/4391)
* plugin/kubernetes: make kubeconfig argument 'context' optional (https://github.com/coredns/coredns/pull/4451)
* plugin/rewrite: copy msg before rewritting (https://github.com/coredns/coredns/pull/4443)
* plugin/sign: track zone file's mtime (https://github.com/coredns/coredns/pull/4431)
* plugin/trace: Use compatible tag name for datadog (https://github.com/coredns/coredns/pull/4408)
* plugin/transfer: only allow outgoing axfr over tcp (https://github.com/coredns/coredns/pull/4452)
