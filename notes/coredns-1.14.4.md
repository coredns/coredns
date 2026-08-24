+++
title = "CoreDNS-1.14.4 Release"
description = "CoreDNS-1.14.4 Release Notes."
tags = ["Release", "1.14.4", "Notes"]
release = "1.14.4"
date = "2026-06-09T00:00:00+00:00"
author = "coredns"
+++

This release improves transport security and operational flexibility, with
enhancements for DoH3 and DoQ, improved DNSSEC signing behavior, and support for
the loong64 architecture. It also adds configurable cache verification, hostname
resolution for forward targets, incoming connection support for dnstap, fallthrough
support in the secondary plugin, automatic zone reloads, and improved forwarding
behavior for NODATA responses.

## Brought to You By

Cedric Wang
Charlie Tonneslan
Dmytro Alieksieiev
Endre Szabo
Immanuel Tikhonov
Isolus
James R T
JUN YANG
Jöran Malek
Nicholas Amorim
Syed Azeez
Umut Polat
Ville Vesilehto
weiguozhang
Yong Tang
徐晓伟

## Noteworthy Changes

* core: Add loong64 arch support (https://github.com/coredns/coredns/pull/8137)
* core: Bound HTTP/3 request header size for DoH3 (https://github.com/coredns/coredns/pull/8135)
* core: Expose TLS ConnectionState (SNI) for DoQ (https://github.com/coredns/coredns/pull/8129)
* core: Remove duplicate cipher suites (https://github.com/coredns/coredns/pull/8118)
* core: Use http.LocalAddrContextKey for DoH local address (https://github.com/coredns/coredns/pull/8149)
* plgin/kubernetes: Remove debug fmt.Println from multicluster zone validation (https://github.com/coredns/coredns/pull/8131)
* plugin/any: Reject invalid any and local config (https://github.com/coredns/coredns/pull/8133)
* plugin/azure: Apply `access` mode to every zone in the same block (https://github.com/coredns/coredns/pull/8110)
* plugin/cache: Add optional verify timeout to serve_stale (https://github.com/coredns/coredns/pull/8070)
* plugin/cache: Allow cache TTLs above default 3600s (https://github.com/coredns/coredns/pull/8134)
* plugin/cache: Prefer positive cache over SERVFAIL in ncache (https://github.com/coredns/coredns/pull/8003)
* plugin/chaos: Reject unknown chaos block options (https://github.com/coredns/coredns/pull/8121)
* plugin/dnssec: Sign each RRset with the zone that owns its name, not the query zone (https://github.com/coredns/coredns/pull/8138)
* plugin/dnstap: Feature: Added incoming connection support (https://github.com/coredns/coredns/pull/8086)
* plugin/file: Canonicalize escape form in owner names (https://github.com/coredns/coredns/pull/8109)
* plugin/file: Trigger reload of zones based on mtime (https://github.com/coredns/coredns/pull/8085)
* plugin/forward: Add hostname resolution support for TO endpoints (https://github.com/coredns/coredns/pull/5646) (https://github.com/coredns/coredns/pull/7923)
* plugin/forward: Forward NODATA responses to Next handler (https://github.com/coredns/coredns/pull/8065)
* plugin/health: Use descriptive error for unknown block options in health and log plugins (https://github.com/coredns/coredns/pull/8128)
* plugin/ready: Reject unknown ready plugin properties (https://github.com/coredns/coredns/pull/8119)
* plugin/proxyproto: Prevent nil pointer dereference when dropping malformed PROXY packets (https://github.com/coredns/coredns/pull/8154)
* plugin/secondary: Add fallthrough support (https://github.com/coredns/coredns/pull/8041)
* plugin/trace: Reject unknown trace and dnstap block options (https://github.com/coredns/coredns/pull/8120)
