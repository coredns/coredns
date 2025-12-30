+++
title = "CoreDNS-1.13.1 Release"
description = "CoreDNS-1.13.1 Release Notes."
tags = ["Release", "1.13.1", "Notes"]
release = "1.13.1"
date = "2025-10-08T00:00:00+00:00"
author = "coredns"
+++

This release updates CoreDNS to Go 1.25.2 and golang.org/x/net v0.45.0 to address multiple
high-severity CVEs. It also improves core performance by avoiding string concatenation in
loops, and hardens the sign plugin by rejecting invalid UTF-8 tokens in dbfile.

## Brought to You By

Catena cyber
Ville Vesilehto
Yong Tang

## Noteworthy Changes

* core: Avoid string concatenation in loops (https://github.com/coredns/coredns/pull/7572)
* core: Update golang to 1.25.2 and golang.org/x/net to v0.45.0 on CVE fixes (https://github.com/coredns/coredns/pull/7598)
* plugin/sign: Reject invalid UTF‑8 dbfile token (https://github.com/coredns/coredns/pull/7589)
