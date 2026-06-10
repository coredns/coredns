+++
title = "CoreDNS-1.14.3 Release"
description = "CoreDNS-1.14.3 Release Notes."
tags = ["Release", "1.14.3", "Notes"]
release = "1.14.3"
date = "2026-03-06T00:00:00+00:00"
author = "coredns"
+++

This release introduces Windows service support, along with full TSIG verification
across DoH, DoH3, QUIC, and gRPC transports, and improved TSIG propagation and DoH
request validation. It also adds optional TLS for the metrics endpoint. Performance
and stability are improved through cache prefetching, QUIC optimizations, and a new
max_age option in the forward plugin. Additional updates include enhanced SVCB/HTTPS
support, improved zone transfer behavior, and various DNSSEC, PROXY protocol, and
concurrency fixes. The release is built with Go 1.26.2, which includes security
fixes addressing CVE-2026-32282, CVE-2026-32289, CVE-2026-33810, CVE-2026-27144,
CVE-2026-27143, CVE-2026-32288, CVE-2026-32283, and CVE-2026-27140, and also includes
fixes for CVE-2026-32936, CVE-2026-33190, CVE-2026-33489, CVE-2026-32934, and CVE-2026-35579.

## Brought to You By

andreyrusanov-ec
cangming
Cedric Wang
Ilya Kulakov
Ingmar Van Glabbeek
John-Michael Mulesa
JUN YANG
liucongran
Minghang Chen
Peppi-Lotta
rpb-ant
Seena Fallah
Syed Azeez
Umut Polat
Ville Vesilehto
Yong Tang

## Noteworthy Changes

* core: Add full TSIG verification in DoH transport (https://github.com/coredns/coredns/pull/8013)
* core: Add full TSIG verification in DoH3 transport (https://github.com/coredns/coredns/pull/8044)
* core: Add full TSIG verification in QUIC transport (ttps://github.com/coredns/coredns/pull/8007)
* core: Add full TSIG verification in gRPC transport (https://github.com/coredns/coredns/pull/8006)
* core: Add support for running CoreDNS as a Windows service (https://github.com/coredns/coredns/pull/7962)
* core: Avoid spawning waiter goroutines when QUIC worker pool is full (https://github.com/coredns/coredns/pull/7927)
* core: Preserve TSIG status in gRPC transport (https://github.com/coredns/coredns/pull/7943)
* core: Propagate TSIG secrets to DoT server (https://github.com/coredns/coredns/pull/7928)
* core: Propagate TSIG status in DoQ transport (https://github.com/coredns/coredns/pull/7947)
* core: Reject oversized GET dns query parameter of DoH (https://github.com/coredns/coredns/pull/7926)
* core: Use per-connection local address for PROXY protocol (https://github.com/coredns/coredns/pull/8005)
* plugin/auto: Resolve symlinked directory before walk (https://github.com/coredns/coredns/pull/8032)
* plugin/cache: Add an atomic.Bool to singleflight prefetching (https://github.com/coredns/coredns/pull/7963)
* plugin/cache: Prefetch without holding a client connection (https://github.com/coredns/coredns/pull/7944)
* plugin/dnssec: Add defensive nil checks (https://github.com/coredns/coredns/pull/7997)
* plugin/dnssec: Avoid caching empty signing results (https://github.com/coredns/coredns/pull/7996)
* plugin/dnssec: Return nil from ParseKeyFile on error (https://github.com/coredns/coredns/pull/8000)
* plugin/dnssec: Return nil sigs on sign error (https://github.com/coredns/coredns/pull/7999)
* plugin/dnsserver: Allow view server blocks in any declaration order (https://github.com/coredns/coredns/pull/8001)
* plugin/file: Expand SVCB/HTTPS record support (https://github.com/coredns/coredns/pull/7950)
* plugin/file: Fix data race in xfr.go (https://github.com/coredns/coredns/pull/8039)
* plugin/file: Introduce snapshot()/setData() accessors for zone data (https://github.com/coredns/coredns/pull/8040)
* plugin/file: Protect Zone.Expired with mutex (https://github.com/coredns/coredns/pull/7940)
* plugin/forward: Add max_age option to enforce an absolute connection lifetime (https://github.com/coredns/coredns/pull/7903)
* plugin/kubernetes: Record cluster_ip services in dns_programming_duration metric (https://github.com/coredns/coredns/pull/7951)
* plugin/kubernetes: Sanitize non-UTF-8 host in metrics (https://github.com/coredns/coredns/pull/7998)
* plugin/metrics: Add optional TLS support to /metrics endpoint (https://github.com/coredns/coredns/pull/7255)
* plugin/metrics: Allow selectively exporting all Go runtime metrics (https://github.com/coredns/coredns/pull/7990)
* plugin/ready: fix Reset list of readiness plugins (https://github.com/coredns/coredns/pull/8035)
* plugin/secondary: Send NOTIFY messages after zone transfer (https://github.com/coredns/coredns/pull/7901)
* plugin/tls: Add the keylog option to configure TLSConfig.KeyLogWriter (https://github.com/coredns/coredns/pull/7537)
* plugin/tls: Use temp dir for keylog test path (https://github.com/coredns/coredns/pull/8010)
* plugin/transfer: Batch AXFR records by message size instead of count (https://github.com/coredns/coredns/pull/8002)
* plugin/transfer: Fix case-sensitive zone handling for AXFR/IXFR (https://github.com/coredns/coredns/pull/7899)
* plugin/transfter: Fix longestMatch to select the most specific zone correctly (https://github.com/coredns/coredns/pull/7949)
* plugin/tsig: Add require_opcode directive for opcode-based TSIG (https://github.com/coredns/coredns/pull/7828)
* proxyproto: Add UDP session tracking for Cloudflare Spectrum PPv2 (https://github.com/coredns/coredns/pull/7967)
