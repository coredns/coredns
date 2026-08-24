+++
title = "CoreDNS-1.14.7 Release"
description = "CoreDNS-1.14.7 Release Notes."
tags = ["Release", "1.14.7", "Notes"]
release = "1.14.7"
date = "2026-07-10T00:00:00+00:00"
author = "coredns"
+++

This release strengthens DNS caching, forwarding, and transport reliability, with
improved stale-cache behavior, safer upstream handling, and tighter connection
and overload controls. It also adds new capabilities for ACME-managed TLS,
topology-aware Kubernetes services, HTTP/2 forwarding, DNS-over-QUIC, and
secondary zone management, while delivering correctness and performance fixes
across file serving, rewrites, ACLs, hosts, transfers, and Kubernetes handling.
The release is built with Go 1.26.6 to include fixes for CVE-2026-56865,
CVE-2026-56864, and CVE-2026-33818.

## Brought to You By

Baltasar Blanco
houyuwushang
Karan V
liucongran
llucas
Manuel Rüger
maximilize
Mehrdad Biukian
Michael Wolf
Ncesam
Nikolaus Schuetz
Nitin Nizhawan
Omkhar Arasaratnam
Pujitha Paladugu
rpb-ant
Saleh
Sueun Cho
Ville Vesilehto
Yash Singh
Yong Tang

## Noteworthy Changes

core: Add connection-level concurrency limiting to DNS-over-QUIC (https://github.com/coredns/coredns/pull/8213)
core: Add max conn limit to https3 (https://github.com/coredns/coredns/pull/8187)
core: Document in-process embedding (https://github.com/coredns/coredns/pull/8436)
core: Make keylog path test portable (https://github.com/coredns/coredns/pull/8311)
core: Normalize server block zones (https://github.com/coredns/coredns/pull/8320)
core: Pin numeric uid/gid for the nonroot user (https://github.com/coredns/coredns/pull/8316)
plugin/acl: Fix autopath from bypassing acl checks (https://github.com/coredns/coredns/pull/8290)
plugin/acl: Fix blocked clients from receiving cached DNS answers (https://github.com/coredns/coredns/pull/8289)
plugin/auto: Fix inverted arguments in duplicate-origin warning (https://github.com/coredns/coredns/pull/8317)
plugin/cache: Add prefer_positive stale policy (https://github.com/coredns/coredns/pull/8378)
plugin/cache: Bind responses and entries to QCLASS (https://github.com/coredns/coredns/pull/8272)
plugin/cache: Configure stale TTL and failure recheck (https://github.com/coredns/coredns/pull/8411)
plugin/cache: Do not cache SOA-less NODATA responses (https://github.com/coredns/coredns/pull/8232)
plugin/cache: Fix cache stale verification metadata race (https://github.com/coredns/coredns/pull/8366)
plugin/cache: Preserve AD when storing cache entries (https://github.com/coredns/coredns/pull/8438)
plugin/cache: Preserve monotonic time for TTL expiry (https://github.com/coredns/coredns/pull/8346)
plugin/file: Do not expand wildcard across a closer empty non-terminal (https://github.com/coredns/coredns/pull/8223)
plugin/file: Fixes multi-primary AXFR zone contamination (https://github.com/coredns/coredns/pull/8367)
plugin/file: Fix panic on zero-valued SOA refresh (https://github.com/coredns/coredns/pull/8276)
plugin/file: Handle empty non-terminal wildcard sources (https://github.com/coredns/coredns/pull/8386)
plugin/file: Resolve each additional section target only once (https://github.com/coredns/coredns/pull/8286)
plugin/file: Return referrals after alias resolution (https://github.com/coredns/coredns/pull/8341)
plugin/file: Run additional processing for CNAME/DNAME answers (https://github.com/coredns/coredns/pull/8337)
plugin/file: Stop self-referential DNAME loops (https://github.com/coredns/coredns/pull/8418)
plugin/forward: Add http(2) host/authority header and TO server resolution (https://github.com/coredns/coredns/pull/8233)
plugin/forward: Cap default connect attempts (https://github.com/coredns/coredns/pull/8365)
plugin/forward: Fast-path string comparison in isAllowedDomain (https://github.com/coredns/coredns/pull/8385)
plugin/forward: Fix incorrect failover counter reset (https://github.com/coredns/coredns/pull/8277)
plugin/forward: Fix incorrect retry of local DNS message serialization failures (https://github.com/coredns/coredns/pull/8313)
plugin/forward: Fix issue in DoH health checks used a default TLS instead of the configured CA (https://github.com/coredns/coredns/pull/8279)
plugin/forward: Fix UDP forwarding so a malformed upstream datagram wont block valid ones later (https://github.com/coredns/coredns/pull/8287)
plugin/hosts: Make unsupported type fallthrough opt-in (https://github.com/coredns/coredns/pull/8282)
plugin/hosts: Pre-convert Origins to plugin.Zones in setup (https://github.com/coredns/coredns/pull/8383)
plugin/kubernetes: Add support for topology-aware headless services via "az-pinned" subdomains (https://github.com/coredns/coredns/pull/8388)
plugin/kubernetes: Add tests for endpoint/service-import equivalence checks (https://github.com/coredns/coredns/pull/8368)
plugin/kubernetes: Copy Labels in Pod.DeepCopyObject (https://github.com/coredns/coredns/pull/8415)
plugin/kubernetes: Pre-allocate search path slice capacity in AutoPath (https://github.com/coredns/coredns/pull/8381)
plugin/kubernetes: Preallocate slice capacities in controller query lookups (https://github.com/coredns/coredns/pull/8343)
plugin/kubernetes: Short-circuit matchPortAndProtocol and fast-path string match (https://github.com/coredns/coredns/pull/8344)
plugin/kubernetes: Skip zone serial bump on DNS neutral pod updates (https://github.com/coredns/coredns/pull/8338)
plugin/proxyproto: Apply an explicitly configured default policy evenwhen no allow list is present. (https://github.com/coredns/coredns/pull/8278)
plugin/rewrite: Normalize exact cname rewrite targets and preserve all records (https://github.com/coredns/coredns/pull/8285)
plugin/rewrite: Preserve original request during rewrites (https://github.com/coredns/coredns/pull/8235)
plugin/rewrite: Test EDNS0 revert with a record present and on the replace path (https://github.com/coredns/coredns/pull/8315)
plugin/secondary: Reset catalog members on ID change (https://github.com/coredns/coredns/pull/8281)
plugin/secondary: Support catalog migration and member scoping (https://github.com/coredns/coredns/pull/8288)
plugin/shed: Add UDP overload protection plugin (https://github.com/coredns/coredns/pull/8312)
plugin/timeouts: Add maxtcpqueries option to bound queries per TCP/TLS connection (https://github.com/coredns/coredns/pull/8376)
plugin/tls: Manage certificates with ACME DNS-01 (https://github.com/coredns/coredns/pull/8310)
plugin/trace: Support IPv6 service endpoints in trace plugin (https://github.com/coredns/coredns/pull/8410)
plugin/transfer: Collect all notify errors instead of shadowing (https://github.com/coredns/coredns/pull/8283)
