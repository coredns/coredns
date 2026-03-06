+++
title = "CoreDNS-1.14.2 Release"
description = "CoreDNS-1.14.2 Release Notes."
tags = ["Release", "1.14.2", "Notes"]
release = "1.14.2"
date = "2026-03-15T00:00:00+00:00"
author = "coredns"
+++

This release adds the new proxyproto plugin to support Proxy Protocol and preserve
client IPs behind load balancers. It also includes enhancements such as improved DNS
logging metadata and stronger randomness for loop detection (CVE-2026-26018), along
with several bug fixes including TLS+IPv6 forwarding, improved CNAME handling and
rewriting, allowing jitter disabling, prevention of an ACL bypass (CVE-2026-26017),
and a Kubernetes plugin crash fix. In addition, the release updates the build to
Go 1.26.1, which include security fixes addressing CVE-2026-27137, CVE-2026-27138, CVE-2026-27139,
CVE-2026-25679, and CVE-2026-27142.

## Brought to You By

Adphi
Henrik Gerdes
hide
Kelly Kane
Shiv Tyagi
vflaux
Ville Vesilehto
yangsenzk
Yong Tang
YOUNEVSKY

## Noteworthy Changes

* core: Reorder rewrite before acl to prevent bypass (https://github.com/coredns/coredns/pull/7882)
* plugin/file: Return SOA and NS records when queried for a record CNAMEd to origin (https://github.com/coredns/coredns/pull/7808)
* plugin/forward: Fix parsing error when handling TLS+IPv6 address (https://github.com/coredns/coredns/pull/7848)
* plugin/log: Add metadata for response Type and Class to Log (https://github.com/coredns/coredns/pull/7806)
* plugin/loop: Use crypto/rand for query name generation (https://github.com/coredns/coredns/pull/7881)
* plugin/kubernetes: Fix panic on empty ListenHosts (https://github.com/coredns/coredns/pull/7857)
* plugin/proxyproto: Add proxy protocol support (https://github.com/coredns/coredns/pull/7738)
* plugin/reload: Allow disabling jitter with 0s (https://github.com/coredns/coredns/pull/7896)
* plugin/rewrite: Fix cname target rewrite for CNAME chains (https://github.com/coredns/coredns/pull/7853)
