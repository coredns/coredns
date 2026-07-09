+++
title = "CoreDNS-1.14.5 Release"
description = "CoreDNS-1.14.5 Release Notes."
tags = ["Release", "1.14.5", "Notes"]
release = "1.14.5"
date = "2026-07-10T00:00:00+00:00"
author = "coredns"
+++

This release improves DNS transport security and operational reliability, with
safer DoH/DoH3 handling, enhanced forwarding configuration, and improved dnstap
support. It also adds robustness improvements across file serving, secondary
zones, transfers, rewrites, hosts handling, and error processing, while fixing
several edge cases in DNS response handling.

## Brought to You By

Aaron Mark
Amirhossein Ebrahimzade
Antoine
Baltasar Blanco
Cedric Wang
Ilya Kulakov
Immanuel Tikhonov
Jaime Hablutzel
Jonathan Tooker
Omkhar Arasaratnam
Pavel Lazureykis
SEONGHYUN HONG
Saleh
Thomas Gosteli
Ville Vesilehto
Yong Tang
houyuwushang
rpb-ant

## Noteworthy Changes

core: Accept scoped IPv6 addresses in transfer targets (https://github.com/coredns/coredns/pull/8204)
core: Bound DoQ stream read with the server read timeout (https://github.com/coredns/coredns/pull/8231)
core: Classify nxdomain without soa as denial (https://github.com/coredns/coredns/pull/8199)
core: Guard Join against an empty label slice (https://github.com/coredns/coredns/pull/8225)
core: Propagate HTTPRequestValidateFunc to all configs in a server block (https://github.com/coredns/coredns/pull/8169)
core: Sanitize DoH/DoH3 request parse errors (https://github.com/coredns/coredns/pull/8254)
core: Use Go TLS defaults (https://github.com/coredns/coredns/pull/8227)
core: Add Config.UDPDecorateWriterFunc for external plugins (https://github.com/coredns/coredns/pull/8257)
plugin/auto: Warn on duplicate zone file origins (https://github.com/coredns/coredns/pull/8191)
plugin/cache: Add regression test for AD bit not partitioning the cache (https://github.com/coredns/coredns/pull/8214)
plugin/dnstap: Close the previous connection before reconnecting (https://github.com/coredns/coredns/pull/8224)
plugin/dnstap: Fix self-deadlock in listener broadcast on client flush error (https://github.com/coredns/coredns/pull/8260)
plugin/dnstap: Store IPv4-mapped IPv6 addresses as 4 octets with SocketFamily INET (https://github.com/coredns/coredns/pull/8186)
plugin/erratic: Apply default truncate amount of 2 for bare `truncate` (https://github.com/coredns/coredns/pull/8240)
plugin/file: Return SOA in authority for negative CNAME target answers (https://github.com/coredns/coredns/pull/8226)
plugin/file: Run additional processing for wildcard answers (https://github.com/coredns/coredns/pull/8222)
plugin/forward: Add doh support (https://github.com/coredns/coredns/pull/8004)
plugin/forward: Make dnstap FORWARDER_* describe the socket from CoreDNS to upstream (https://github.com/coredns/coredns/pull/8184)
plugin/forward: Make per-upstream read timeout configurable (https://github.com/coredns/coredns/pull/8205)
plugin/forward: Restore old behavior forward plugin continue on empty conf file (https://github.com/coredns/coredns/pull/8203)
plugin/hosts: Add wildcard support (https://github.com/coredns/coredns/pull/8185)
plugin/hosts: Fall through unsupported query types (https://github.com/coredns/coredns/pull/8193)
plugin/hosts: Fix data race between lookups and reload (https://github.com/coredns/coredns/pull/8253)
plugin/kubernetes: Fix AXFR panic when nsAddrs returns multiple records (https://github.com/coredns/coredns/pull/8256)
plugin/local: Handle names under .localhost. (https://github.com/coredns/coredns/pull/8151)
plugin/log: Synthesize deferred error responses (https://github.com/coredns/coredns/pull/8200)
plugin/rewrite: Fix nil-pointer panic in EDNS0 response reversion with no OPT record (https://github.com/coredns/coredns/pull/8190)
plugin/rewrite: Restore the original question on empty replies (https://github.com/coredns/coredns/pull/8212)
plugin/secondary: Parse catalog zones after transfer (https://github.com/coredns/coredns/pull/8209)
plugin/secondary: Stop update loop on reload shutdown (https://github.com/coredns/coredns/pull/8198)
plugin/trace: Correct Zipkin v2 endpoint docs (https://github.com/coredns/coredns/pull/8202)
plugin/transfer: Configure notify source address (https://github.com/coredns/coredns/pull/8192)
plugin/transfer: Fix panic in CoreDNS transfer plugin caused by empty DNS record (https://github.com/coredns/coredns/pull/8207)
plugin/tsig: Don't echo client's TSIG.Error if verification is successful (https://github.com/coredns/coredns/pull/8215)
