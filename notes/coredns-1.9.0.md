+++
title = "CoreDNS-1.9.0 Release"
description = "CoreDNS-1.9.0 Release Notes."
tags = ["Release", "1.9.0", "Notes"]
release = "1.9.0"
date = "2022-02-01T00:00:00+00:00"
author = "coredns"
+++

This is a release with bug fixes and some new features added. Starting with 1.9.0
the minimal required go version will be 1.17.

## Brought to You By

Chris O'Haver,
Ondřej Benkovský,
Yong Tang,
xuweiwei

## Noteworthy Changes

* plugin/prometheus: Write rcode properly to the metrics (https://github.com/coredns/coredns/pull/5126)
* plugin/template: Persist truncated state to client if CNAME lookup response is truncated (https://github.com/coredns/coredns/pull/4713)
