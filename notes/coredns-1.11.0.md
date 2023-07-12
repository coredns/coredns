+++
title = "CoreDNS-1.11.0 Release"
description = "CoreDNS-1.11.0 Release Notes."
tags = ["Release", "1.11.0", "Notes"]
release = "1.11.0"
date = "2023-06-02T00:00:00+00:00"
author = "coredns"
+++

## Brought to You By

    27	Amila Senadheera
    26	Ayato Tokubi
    25	Ben Kochie
    24	Catena cyber
    23	Chris O'Haver
    22	Dan Salmon
    21	Denis MACHARD
    20	Fish-pro
    19	Gabor Dozsa
    18	Gary McDonald
    17	Justin
    16	Lio李歐
    15	Marcos Mendez
    14	Pat Downey
    13	Rotem Kfir
    12	Sebastian Dahlgren
    11	Vancl
    10	Vinayak Goyal
     9	W. Trevor King
     8	Yash Singh
     7	Yashpal
     6	Yong Tang
     5	cui fliter
     4	dependabot[bot]
     3	jeremiejig
     2	junhwong
     1	yyzxw

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
