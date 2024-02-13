+++
title = "CoreDNS-1.11.0 Release"
description = "CoreDNS-1.11.0 Release Notes."
tags = ["Release", "1.11.0", "Notes"]
release = "1.11.0"
date = "2023-07-25T00:00:00+00:00"
author = "coredns"
+++

## Release Highlights

* Adds support for accepting DNS connections over QUIC (doq).
* Adds CNAME target rewrites to the _rewrite_ plugin.
* Plus many bug fixes, and some security improvements.

This release introduces the following backward incompatible changes:
* In the _kubernetes_ plugin, we have dropped support for watching Endpoint and Endpointslice v1beta, since all supported K8s versions now use Endpointslice.
* The _bufsize_ plugin changed its default size limit value to 1232
* Some changes to _forward_ plugin metrics.
## Brought to You By

Amila Senadheera,
Antony Chazapis,
Ayato Tokubi,
Ben Kochie,
Catena cyber,
Chris O'Haver,
Dan Salmon,
Dan Wilson,
Denis MACHARD,
Diogenes Pelisson,
Eng Zer Jun,
Fish-pro,
Gabor Dozsa,
Gary McDonald,
João Henri,
Justin,
Lio李歐,
Marcos Mendez,
Marius Kimmina,
Ondřej Benkovský,
Pat Downey,
Petr Menšík,
Rotem Kfir,
Sebastian Dahlgren,
Vancl,
Vinayak Goyal,
W. Trevor King,
Yash Singh,
Yashpal,
Yong Tang,
Yuheng,
cui fliter,
jeremiejig,
junhwong,
rokkiter,
yyzxw

## Other Noteworthy Changes

* add support for RISC-V (https://github.com/coredns/coredns/pull/6195)
* doh: allow http as the protocol (https://github.com/coredns/coredns/pull/5762)
* doq: add DNS-Over-QUIC server support (https://github.com/coredns/coredns/pull/6182)
* plugin/bufsize: change default value to 1232 (https://github.com/coredns/coredns/pull/6183)
* plugin/clouddns: fix answers limited to one response (https://github.com/coredns/coredns/pull/5986)
* plugin/dnssec: on delegation, sign DS or NSEC of no DS. (https://github.com/coredns/coredns/pull/5899)
* plugin/dnstap: add tls support (https://github.com/coredns/coredns/pull/5917)
* plugin/forward: continue waiting after receiving malformed responses (https://github.com/coredns/coredns/pull/6014)
* plugin/forward: fix forward metrics for backwards compatibility (https://github.com/coredns/coredns/pull/6178)
* plugin/health: poll localhost by default (https://github.com/coredns/coredns/pull/5934)
* plugin/k8s_external: add fallthrough option (https://github.com/coredns/coredns/pull/5959)
* plugin/kubernetes: expose client-go internal request metrics (https://github.com/coredns/coredns/pull/5991)
* plugin/kubernetes: filter ExternalName service queries for subdomains of subdomains (https://github.com/coredns/coredns/pull/6162)
* plugin/kubernetes: fix headless/endpoint query panics when endpoints are disabled (https://github.com/coredns/coredns/pull/6137)
* plugin/kubernetes: fix ports panic (https://github.com/coredns/coredns/pull/6179)
* plugin/kubernetes: remove Endpoint and EndpointSlice v1beta Support (https://github.com/coredns/coredns/pull/6147)
* plugin/loadbalance: improve weights update (https://github.com/coredns/coredns/pull/5906)
* plugin/rewrite: introduce cname target rewrite rule to rewrite plugin (https://github.com/coredns/coredns/pull/6004)
* plugin/transfer: send notifies after adding zones all zones (https://github.com/coredns/coredns/pull/5774)
* prevent fail counter of a proxy overflows (https://github.com/coredns/coredns/pull/5990)
* prevent panics when using DoHWriter (https://github.com/coredns/coredns/pull/6120)
* run coredns as non root. (https://github.com/coredns/coredns/pull/5969)
* support unix socket for GRPC (https://github.com/coredns/coredns/pull/5943)
