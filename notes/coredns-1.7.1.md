+++
title = "CoreDNS-1.7.1 Release"
description = "CoreDNS-1.7.1 Release Notes."
tags = ["Release", "1.7.1", "Notes"]
release = "1.7.1"
date = 2020-06-15T10:00:00+00:00
author = "coredns"
draft = true
+++

**UNRELEASED**

The CoreDNS team has released
[CoreDNS-1.7.1](https://github.com/coredns/coredns/releases/tag/v1.7.1).

This is a small, incremental release that adds some polish.

Note that the next (or at least soon), we will

* merge caddy v1 into the coredns org. This requires a import renaming for all plugins: ` sed -i
 's|github.com/caddyserver/caddy|github.com/coredns/caddy|'`, i.e. "github.com/caddyserver/caddy"
  needs to be renamed to "github.com/coredns/caddy".
* merge the new transfer plugin and move plugins over to that; if you don't use the *file* plugin,
  nor zonetransfer this doesn't apply to you.

## Brought to You By


## Noteworthy Changes

* core: Add timeouts for http server (https://github.com/coredns/coredns/pull/3920).
* plugin/forward: Register HealthcheckBrokenCount (https://github.com/coredns/coredns/pull/4021).
* plugin/grpc: Improve gRPC Plugin when backend is not available (https://github.com/coredns/coredns/pull/3966)
* plugins: Using promauto package to ensure all created metrics are properly registered (https://github.com/coredns/coredns/pull/4025).
* plugin/trace: Only with *debug* active enable debug mode for tracing - removes extra logging (https://github.com/coredns/coredns/pull/4016).
* project: Add DCO requirement in Contributing guidelines (https://github.com/coredns/coredns/pull/4008).
