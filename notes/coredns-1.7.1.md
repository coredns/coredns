+++
title = "CoreDNS-1.7.1 Release"
description = "CoreDNS-1.7.1 Release Notes."
tags = ["Release", "1.7.1", "Notes"]
release = "1.7.1"
date = 2020-06-15T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.7.1](https://github.com/coredns/coredns/releases/tag/v1.7.1).

This is a small, incremental release.

## Brought to You By


## Noteworthy Changes

* plugin/forward: register HealthcheckBrokenCount (https://github.com/coredns/coredns/pull/4021).
  See PR TBD for using `promauto` to never have this 'forget to register metric' again.
* plugin/trace: Only with *debug* active enable  debug mode for tracing - removes extra logging (https://github.com/coredns/coredns/pull/4016)
* plugin/grpc: Improve gRPC Plugin when backend is not available (https://github.com/coredns/coredns/pull/3966)
* project: Add DCO requirement in Contributing guidelines (https://github.com/coredns/coredns/pull/4008)
* core: Add timeouts for http server (https://github.com/coredns/coredns/pull/3920)
