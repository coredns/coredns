+++
title = "CoreDNS-1.11.0 Release"
description = "CoreDNS-1.11.0 Release Notes."
tags = ["Release", "1.11.0", "Notes"]
release = "1.11.0"
date = "2023-07-25T00:00:00+00:00"
author = "coredns"
+++

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
Eng Zer Jun,
Fish-pro,
Gabor Dozsa,
Gary McDonald,
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
cui fliter,
jeremiejig,
junhwong,
rokkiter,
yyzxw

## Noteworthy Changes

* add support unix socket for GRPC (https://github.com/coredns/coredns/pull/5943)
* plugin/forward: Continue waiting after receiving malformed responses (https://github.com/coredns/coredns/pull/6014)
* plugin/dnssec: on delegation, sign DS or NSEC of no DS. (https://github.com/coredns/coredns/pull/5899)
* plugin/kubernetes: expose client-go internal request metrics (https://github.com/coredns/coredns/pull/5991)
* Prevent fail counter of a proxy overflows (https://github.com/coredns/coredns/pull/5990)
* plugin/rewrite: Introduce cname target rewrite rule to rewrite plugin (https://github.com/coredns/coredns/pull/6004)
* plugin/health: Poll localhost by default (https://github.com/coredns/coredns/pull/5934)
* plugin/k8s_external: Supports fallthrough option (https://github.com/coredns/coredns/pull/5959)
* plugin/clouddns: fix answers limited to one response (https://github.com/coredns/coredns/pull/5986)
* Run coredns as non root. (https://github.com/coredns/coredns/pull/5969)
* DoH: Allow http as the protocol (https://github.com/coredns/coredns/pull/5762)
* plugin/dnstap: tls support (https://github.com/coredns/coredns/pull/5917)
* plugin/transfer: send notifies after adding zones all zones (https://github.com/coredns/coredns/pull/5774)
* plugin/loadbalance: Improve weights update (https://github.com/coredns/coredns/pull/5906)
