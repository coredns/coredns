+++
title = "CoreDNS-1.7.0 Release"
description = "CoreDNS-1.7.0 Release Notes."
tags = ["Release", "1.7.0", "Notes"]
release = "1.7.0"
date = 2020-06-15T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.7.0](https://github.com/coredns/coredns/releases/tag/v1.7.0).

This is a **backwards-incompatible release**. Major changes include:

* Better [metrics names](https://github.com/coredns/coredns/pull/3776).
* The *federation* plugin (which allows for v1 Kubernetes federation) has been removed. We've also removed
  some supporting code from the *kubernetes* plugin, so it will not build as an external plugin
  (with this version of CoreDNS).

As this was already backwards-incompatible release, we took the liberty of stuffing as much in
one release as possible to minimize the disruption going forward.

A new plugin, [*dns64*](https://coredns.io/plugins/dns64), was promoted from external to a plugin that
is included by default. This plugin "enables DNS64 IPv6 transition mechanism."

### Metric Changes

We mostly dropped `count` from `_total` metrics names:

* `coredns_request_block_count_total` -\> `coredns_dns_blocked_requests_total`
* `coredns_request_allow_count_total` -\> `coredns_dns_allowed_requests_total`

* `coredns_dns_acl_request_block_count_total` -\> `coredns_acl_blocked_requests_total`
* `coredns_dns_acl_request_allow_count_total` -\> `coredns_acl_allowed_requests_total`

* `coredns_autopath_success_count_total` -\> `coredns_autopath_success_total`

* `coredns_forward_request_count_total` -\> `coredns_forward_requests_total`
* `coredns_forward_response_rcode_count_total` -\> `coredns_forward_responses_total`
* `coredns_forward_healthcheck_failure_count_total` -\> `coredns_forward_healthcheck_failures_total`
* `coredns_forward_healthcheck_broken_count_total` -\> `coredns_forward_healthcheck_broken_total`
* `coredns_forward_max_concurrent_reject_count_total` -\> `coredns_forward_max_concurrent_rejects_total`

* `coredns_grpc_request_count_total` -\> `coredns_grpc_requests_total`
* `coredns_grpc_response_rcode_count_total` -\> `coredns_grpc_responses_total`

* `coredns_panic_count_total` -\> `coredns_panics_total`
* `coredns_dns_request_count_total` -\> `coredns_dns_requests_total`
* `coredns_dns_request_do_count_total` -\> `coredns_dns_do_requests_total`
* `coredns_dns_response_rcode_count_total` -\> `coredns_dns_responses_total`

* `coredns_reload_failed_count_total` -\> `coredns_reload_failed_total`

* `coredns_cache_size` -\> `coredns_cache_entries`

And note that
`coredns_dns_request_type_count_total` is now part of `coredns_dns_requests_total` .

## Brought to You By

Ambrose Chua,
Ben Kochie,
Catena cyber,
Chanakya-Ekbote,
Chris O'Haver,
Daisuke TASAKI,
Eli Lindsey,
Erfan Besharat,
Krzysztof Dąbrowski,
Michael Kashin,
Miek Gieben,
Mirek S,
Pablo Caderno,
Sandeep Rajan,
Tobias Schmidt,
Yang Bo,
Yong Tang,
Zou Nengren.

## Noteworthy Changes

* plugin/azure: Fix environment option overwrite (https://github.com/coredns/coredns/pull/3922)
* plugin/dns64: Add DNS64 plugin (https://github.com/coredns/coredns/pull/3534)
* plugin/federation: Remove already-deprecated federation plugin (https://github.com/coredns/coredns/pull/3794)
* plugin/forward: Fix only first upstream server is used in forward plugin (https://github.com/coredns/coredns/issues/3900)
* plugin/forward: Avoid https protocol (https://github.com/coredns/coredns/pull/3817)
* plugin/k8s_external: Add CNAME support for AWS ELB/NLB (https://github.com/coredns/coredns/pull/3916)
* plugin/kubernetes: Remove already-deprecated options `resyncperiod` and `upstream` (https://github.com/coredns/coredns/pull/3737)
* plugin/kubernetes: Populate client metadata for external queries (https://github.com/coredns/coredns/pull/3874)
* plugin/kubernetes: Fix 0 weight in SRV records with 100 or more records in answer (https://github.com/coredns/coredns/pull/3931)
* plugin/kubernetes: Handle tombstones in kubernetes plugin (https://github.com/coredns/coredns/pull/3887) and (https://github.com/coredns/coredns/pull/3890)
* plugin/nsid: Fix NSID not being set on cached responses (https://github.com/coredns/coredns/pull/3822)
* metrics: Better metrics names (https://github.com/coredns/coredns/pull/3776), (https://github.com/coredns/coredns/pull/3799), and (https://github.com/coredns/coredns/pull/3805)
