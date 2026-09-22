+++
title = "CoreDNS-1.14.6 Release"
description = "CoreDNS-1.14.6 Release Notes."
tags = ["Release", "1.14.6", "Notes"]
release = "1.14.6"
date = "2026-07-10T00:00:00+00:00"
author = "coredns"
+++

This patch release focuses on fixing ARM and MIPS build issues introduced in
v1.14.5 by downgrading the dd-trace-go dependency, while also including
improvements to forwarding and secondary zone support.

## Brought to You By

Filippo125
houyuwushang
Immanuel Tikhonov
Ville Vesilehto
Yong Tang

## Noteworthy Changes

core: Downgrade dd-trace-go to v2.8.2 (https://github.com/coredns/coredns/pull/8266)
plugin/auto: Keep first matching zone file for duplicate origins (https://github.com/coredns/coredns/pull/8216)
plugin/forward: Add source_address directive (https://github.com/coredns/coredns/pull/8011)
plugin/secondary: Serve catalog member zones (https://github.com/coredns/coredns/pull/8230)
