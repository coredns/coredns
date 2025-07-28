+++
title = "CoreDNS-1.12.3 Release"
description = "CoreDNS-1.12.3 Release Notes."
tags = ["Release", "1.12.3", "Notes"]
release = "1.12.3"
date = "2024-07-30T00:00:00+00:00"
author = "coredns"
+++

This release improves plugin reliability and standards compliance, adding startup timeout to the Kubernetes
plugin, fallthrough to gRPC, and EDNS0 unset to rewrite. The file plugin now preserves SRV record case per
RFC 6763, route53 is updated to AWS SDK v2, and multiple race conditions in cache and connection handling in
forward are fixed.

## Brought to You By

blakebarnett
Cameron Steel
Dave Brown
Dennis Simmons
harshith-2411-2002
houpo-bob
Oleg Guba
Sebastian Mayr
Stephen Kitt
Syed Azeez
Ville Vesilehto
Yong Tang
Yoofi Quansah


## Noteworthy Changes

* plugin/auto: Return REFUSED when no next plugin is available (https://github.com/coredns/coredns/pull/7381)
* plugin/cache: Create a copy of a response to ensure original msg is never modified (https://github.com/coredns/coredns/pull/7357)
* plugin/cache: Fix data race when refreshing cached messages (https://github.com/coredns/coredns/pull/7398)
* plugin/cache: Fix data race when updating the TTL of cached messages (https://github.com/coredns/coredns/pull/7397)
* plugin/file: Return REFUSED when no next plugin is available (https://github.com/coredns/coredns/pull/7381)
* plugin/file: Preserve case in SRV record names and targets per RFC 6763 (https://github.com/coredns/coredns/pull/7402)
* plugin/forward: Handle cached connection closure in forward plugin (https://github.com/coredns/coredns/pull/7427)
* plugin/grpc: Add support for fallthrough to the grpc plugin (https://github.com/coredns/coredns/pull/7359)
* plugin/kubernetes: Add startup_timeout for kubernetes plugin (https://github.com/coredns/coredns/pull/7068)
* plugin/rewrite: Add EDNS0 unset action (https://github.com/coredns/coredns/pull/7380)
* plugin/route53: Port to AWS Go SDK v2 (https://github.com/coredns/coredns/pull/6588)
* plugin/test: Fix TXT record comparison logic for multi-string vs multi-record scenarios (https://github.com/coredns/coredns/pull/7413)
