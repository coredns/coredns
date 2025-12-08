+++
title = "CoreDNS-1.13.2 Release"
description = "CoreDNS-1.13.2 Release Notes."
tags = ["Release", "1.13.2", "Notes"]
release = "1.13.2"
date = "2025-12-08T00:00:00+00:00"
author = "coredns"
+++

This release adds initial support for DoH3 and includes several core performance and stability
fixes, including reduced allocations, a resolved data race in uniq, and safer QUIC listener
initialization. Plugin updates improve forwarder reliability, extend GeoIP schema support,
and fix issues in secondary, nomad, and kubernetes. Cache and file plugins also receive
targeted performance tuning.

## Brought to You By

Alicia Y
Andrey Smirnov
Brennan Kinney
Charlie Vieth
Endre Szabo
Eric Case
Filippo125
Nico Berlee
Olli Janatuinen
Rick Fletcher
Timur Solodovnikov
Tomas Boros
Ville Vesilehto
cangming
rpb-ant
wencyu
wenxuan70
Yong Tang
zhetaicheleba

## Noteworthy Changes

* core: Add basic support for DoH3 (https://github.com/coredns/coredns/pull/7677)
* core: Avoid proxy unnecessary alloc in Yield (https://github.com/coredns/coredns/pull/7708)
* core: Fix usage of sync.Pool to save an alloc (https://github.com/coredns/coredns/pull/7701)
* core: Fix data race with sync.RWMutex for uniq (https://github.com/coredns/coredns/pull/7707)
* core: Prevent QUIC reload panic by lazily initializing the listener (https://github.com/coredns/coredns/pull/7680)
* core: Refactor/use reflect.TypeFor (https://github.com/coredns/coredns/pull/7696)
* plugin/auto: Limit regex length (https://github.com/coredns/coredns/pull/7737)
* plugin/cache: Remove superfluous allocations in item.toMsg (https://github.com/coredns/coredns/pull/7700)
* plugin/cache: Isolate metadata in prefetch goroutine (https://github.com/coredns/coredns/pull/7631)
* plugin/cache: Correct spelling of MaximumDefaultTTL in cache and dnsutil packages (https://github.com/coredns/coredns/pull/7678)
* plugin/dnstap: Better error handling (redial & logging) when Dnstap is busy (https://github.com/coredns/coredns/pull/7619)
* plugin/file: Performance finetuning (https://github.com/coredns/coredns/pull/7658)
* plugin/forward: Disallow NOERROR in failover (https://github.com/coredns/coredns/pull/7622)
* plugin/forward: Added support for per-nameserver TLS SNI (https://github.com/coredns/coredns/pull/7633)
* plugin/forward: Prevent busy loop on connection err (https://github.com/coredns/coredns/pull/7704)
* plugin/forward: Add max connect attempts knob (https://github.com/coredns/coredns/pull/7722)
* plugin/geoip: Add ASN schema support (https://github.com/coredns/coredns/pull/7730)
* plugin/geoip: Add support for subdivisions (https://github.com/coredns/coredns/pull/7728)
* plugin/kubernetes: Fix kubernetes plugin logging (https://github.com/coredns/coredns/pull/7727)
* plugin/multisocket: Cap num sockets to prevent OOM (https://github.com/coredns/coredns/pull/7615)
* plugin/nomad: Support service filtering (https://github.com/coredns/coredns/pull/7724)
* plugin/rewrite: Pre-compile CNAME rewrite regexp (https://github.com/coredns/coredns/pull/7697)
* plugin/secondary: Fix reload causing secondary plugin goroutine to leak (https://github.com/coredns/coredns/pull/7694)
