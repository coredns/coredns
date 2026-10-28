+++
title = "CoreDNS-1.15.0 Release"
description = "CoreDNS-1.15.0 Release Notes."
tags = ["Release", "1.15.0", "Notes"]
release = "1.15.0"
date = "2026-10-20T00:00:00+00:00"
author = "coredns"
+++

This release improves DNS caching, forwarding, and transport reliability,
with fixes for stale-cache handling, upstream connections, crashes/race fixes,
as well as enhancemence from quite a few plugins.

Forwarding now supports DNS-over-QUIC upstreams. Additional new features include
optional JSON query logging, authenticated RFC 2136 dynamic DNS updates, SIIT support,
expr-lang templates, and PROXY protocol v2 metadata.

The release is built with Go 1.27.3, covering CVE-2026-56865, CVE-2026-56864,
CVE-2026-33818, CVE-2026-97032, CVE-2026-78659, CVE-2026-97031, and CVE-2026-111943, along with
other security fixes.

## Brought to You By

Ahmad Mehrabi
Amila Senadheera
Andri Yngvason
Aradhya Jain
Arunesh Dwivedi
Baltasar Blanco
Ben Kochie
Choudhry Shehryar
Christian Menges
houyuwushang
Ilya Kulakov
jxj
llucas
Manuel Rüger
Michée lengronne
MohibShaikh
myohannes
Paco Cartones
Pujitha Paladugu
Sakıp Han Dursun
Saleh
sam lockart
ump45nose
Yash Singh
Yong Tang
Zhao Jianing

## Noteworthy Changes

core: Add opt-in JSON logging with structured DNS query fields (https://github.com/coredns/coredns/pull/8553)
core: Add opt-in opcode admission (https://github.com/coredns/coredns/pull/8469)
core: Cancel only the stalled DoQ stream, not the whole DoQ connection (https://github.com/coredns/coredns/pull/8567)
core: Fix DataDog library dependency issue (https://github.com/coredns/coredns/pull/8541)
core: Make TsigSecret public (https://github.com/coredns/coredns/pull/8434)
core: Reject conflicting TLS policies on shared listeners. (https://github.com/coredns/coredns/pull/8565)
core: Stop echoing unhandled EDNS options (https://github.com/coredns/coredns/pull/8514)
core: Support explicit registration for embedded hosts (https://github.com/coredns/coredns/pull/8525)
core: Use Zones.Contains when any match suffices (https://github.com/coredns/coredns/pull/8505)
plugin/autopath: Fixes a nil pointer dereference panic in autopath during search path walk (https://github.com/coredns/coredns/pull/8517)
plugin/azure: Add refresh directive (https://github.com/coredns/coredns/pull/8614)
plugin/azure: Allow startup with unavailable zones (https://github.com/coredns/coredns/pull/8524)
plugin/cache: Cache SERVFAIL when the resolver fails without a response (https://github.com/coredns/coredns/pull/8629)
plugin/cache: Clarify stale verify behavior (https://github.com/coredns/coredns/pull/8579)
plugin/cache: Don't cache or panic on responses with no question (https://github.com/coredns/coredns/pull/8467)
plugin/cache: Fix timed stale caused refreshes (https://github.com/coredns/coredns/pull/8587)
plugin/cache: Prevents a nil pointer dereference panic in the cache prefetch (https://github.com/coredns/coredns/pull/8512)
plugin/cache: Stop setting AA on answers served from cache (https://github.com/coredns/coredns/pull/8419)
plugin/clouddns: Add endpoint directive (https://github.com/coredns/coredns/pull/8618)
plugin/clouddns: Add refresh directive (https://github.com/coredns/coredns/pull/8616)
plugin/dns64: Fixes a nil pointer dereference panic in dns64 during response (https://github.com/coredns/coredns/pull/8511)
plugin/dns64: Preserve A fallback response codes (https://github.com/coredns/coredns/pull/8619)
plugin/dns64: Reject unsupported prefix lengths (https://github.com/coredns/coredns/pull/8611)
plugin/dnstap: Tap deferred error responses (https://github.com/coredns/coredns/pull/8549)
plugin/dynupdate: Add durable authenticated RFC 2136 updates (https://github.com/coredns/coredns/pull/8520)
plugin/etcd: Allow disabling legacy apex fallback (https://github.com/coredns/coredns/pull/8468)
plugin/etcd: Avoid apex fallback on backend errors (https://github.com/coredns/coredns/pull/8465)
plugin/etcd: Fix etcd library update issue (https://github.com/coredns/coredns/pull/8542)
plugin/etcd: Stop target stripping at the end of a name (https://github.com/coredns/coredns/pull/8584)
plugin/file: Fix less is not up to RFC 1034 and 4034 (https://github.com/coredns/coredns/pull/8503)
plugin/file: Fix race condition in file plugin closer (https://github.com/coredns/coredns/pull/8583)
plugin/file: Reject SOA owners that do not match the zone (https://github.com/coredns/coredns/pull/8555)
plugin/file: Return SERVFAIL on self-referential CNAME loops (https://github.com/coredns/coredns/pull/8475)
plugin/file: Test that out-of-zone CNAME targets keep NOERROR (https://github.com/coredns/coredns/pull/8569)
plugin/file: Track zone mtime for reload_by_mtime (https://github.com/coredns/coredns/pull/8556)
plugin/forward: Bound DoT connection setup for retries (https://github.com/coredns/coredns/pull/8543)
plugin/forward: Ensure forward sets the DoH host (https://github.com/coredns/coredns/pull/8470)
plugin/forward: Set IP_RECVERR on upstream UDP sockets (https://github.com/coredns/coredns/pull/8628)
plugin/forward: Support DNS-over-QUIC upstreams (https://github.com/coredns/coredns/pull/8474)
plugin/forward: Treat forward upstream hostnames as absolute FQDNs to avoid search-domain resolution. (https://github.com/coredns/coredns/pull/8563)
plugin/grpc: Close clients on shutdown (https://github.com/coredns/coredns/pull/8582)
plugin/grpc_server: Both limits accept zero, not only positive integers (https://github.com/coredns/coredns/pull/8660)
plugin/hosts: Don't drop entries after an over-long line (https://github.com/coredns/coredns/pull/8496)
plugin/hosts: Don't drop over-long fields silently (https://github.com/coredns/coredns/pull/8551)
plugin/hosts: Parse hosts files with bufio.Reader (https://github.com/coredns/coredns/pull/8516)
plugin/https: Add max_streams to limit HTTP/2 concurrent streams (https://github.com/coredns/coredns/pull/8522)
plugin/k8s_external: Correct the SOA example in the docs (https://github.com/coredns/coredns/pull/8666)
plugin/k8s_external: Fixes a nil pointer dereference panic when upstream lookup returns no response (https://github.com/coredns/coredns/pull/8518)
plugin/kubernetes: Include structured key/value context in client-go logs (https://github.com/coredns/coredns/pull/8490)
plugin/kubernetes: Isolate NS address test fixtures (https://github.com/coredns/coredns/pull/8513)
plugin/kubernetes: Support pods with multiple IPs (https://github.com/coredns/coredns/pull/8635)
plugin/loadbalance: Reduce roundRobin allocations for homogeneous address sets (https://github.com/coredns/coredns/pull/8375)
plugin/loadbalance: Validate the response before dereferencing it in WriteMsg (https://github.com/coredns/coredns/pull/8523)
plugin/metrics: Drop the nonexistent type label from request_duration_seconds (https://github.com/coredns/coredns/pull/8650)
plugin/metrics: Release listener on TLS startup failure (https://github.com/coredns/coredns/pull/8527)
plugin/minimal: Fixes a nil pointer dereference panic in minimal prefetch response processing (https://github.com/coredns/coredns/pull/8506)
plugin/proxyproto: Expose PPv2 TLVs to downstream plugins as metadata (https://github.com/coredns/coredns/pull/8641)
plugin/proxyproto: Preserve the UDP peer for LOCAL (https://github.com/coredns/coredns/pull/8581)
plugin/quic: Add max_connections directive (https://github.com/coredns/coredns/pull/8610)
plugin/ready: Release lock before server shutdown (https://github.com/coredns/coredns/pull/8526)
plugin/reload: Broadcast shutdown signal (https://github.com/coredns/coredns/pull/8504)
plugin/rewrite: Drop the per-record dot-prefixed temporary in remapStringRewriter (https://github.com/coredns/coredns/pull/8382)
plugin/rewrite: Fixes a nil pointer dereference panic in ResponseReverter.WriteMsg (https://github.com/coredns/coredns/pull/8519)
plugin/rewrite: Limit rewrite cname recursion (https://github.com/coredns/coredns/pull/8570)
plugin/rewrite: Point to config instead of copying for CNAME rewrites (https://github.com/coredns/coredns/pull/8568)
plugin/rewrite: Support opt-in metadata in regex name replacements (https://github.com/coredns/coredns/pull/8665)
plugin/route53: Don't hold zMu across zone Lookup (https://github.com/coredns/coredns/pull/8601)
plugin/route53: Fix 'crentials' typo in README and generated man page (https://github.com/coredns/coredns/pull/8655)
plugin/siit: Add initial SIIT implementation (https://github.com/coredns/coredns/pull/8188)
plugin/template: Add expr-lang support to templates (https://github.com/coredns/coredns/pull/8450)
plugin/tls: Document that dig supports DoT (https://github.com/coredns/coredns/pull/8539)
plugin/trace: Recognize Zipkin URL schemes by prefix (https://github.com/coredns/coredns/pull/8625)
plugin/trace: Return error from Zipkin NewEndpoint in trace setup (https://github.com/coredns/coredns/pull/8577)
plugin/tsig: Expose validated TSIG key identity (https://github.com/coredns/coredns/pull/8471)
plugin/tsig: Reject non-final TSIG records (https://github.com/coredns/coredns/pull/8585)
plugin/view: Drop a stray closing brace from the split-DNS example (https://github.com/coredns/coredns/pull/8643)
