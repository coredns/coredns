+++
title = "CoreDNS-1.14. Release"
description = "CoreDNS-1.14.1 Release Notes."
tags = ["Release", "1.14.1", "Notes"]
release = "1.14.1"
date = "2026-01-15T00:00:00+00:00"
author = "coredns"
+++

This release primarily addresses security vulnerabilities affecting Go versions prior to
Go 1.25.6 and Go 1.24.12 (CVE-2025-61728, CVE-2025-61726, CVE-2025-68121, CVE-2025-61731,
CVE-2025-68119). It also includes performance improvements to the proxy plugin via
multiplexed connections, along with various documentation updates.

## Brought to You By

Alex Massy
Shiv Tyagi
Ville Vesilehto
Yong Tang

## Noteworthy Changes

* plugin/proxy: Use mutex-based connection pool (https://github.com/coredns/coredns/pull/7790)
