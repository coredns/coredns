+++
title = "CoreDNS-1.11.4 Release"
description = "CoreDNS-1.11.4 Release Notes."
tags = ["Release", "1.11.4", "Notes"]
release = "1.11.4"
date = "2024-11-13T00:00:00+00:00"
author = "coredns"
+++

This release adds some new features and fixes some bugs.  New features of note:
  * forward plugin: new option `next`, to try alternate upstreams when receiving specified response codes upstreams on (functions like the external plugin _alternate_) 
  * dnssec plugin: new option to load keys from AWS Secrets Manager
  * rewrite plugin: new option to revert EDNS0 option rewrites in responses

## Brought to You By

AdamKorcz,
Anifalak,
Ben Kochie,
Chris O'Haver,
Frederic Hemery,
Grant Spence,
Harshita Sao,
Jason Joo,
Jasper Bernhardt,
Johnny Bergström,
Keith Coleman,
Kevin Lyda,
Lan,
Lin-1997,
Manuel Rüger,
Nathan Currier,
Nicolai Søborg,
Nikita Usatov,
Paco Xu,
Reinhard Nägele,
Robbie Ostrow,
TAKAHASHI Shuuji,
Till Riedel,
Tobias Klauser,
YASH JAIN,
cedar-gao,
chenylh,
wmkuipers,
xinbenlv,
zhangguanzhang

## Noteworthy Changes

* core: set cache-control max-age as integer, not float (https://github.com/coredns/coredns/pull/6764)
* plugin/metadata: evaluate metadata in plugin order (https://github.com/coredns/coredns/pull/6729)
* plugin/dnssec: dnssec load keys from AWS Secrets Manager (https://github.com/coredns/coredns/pull/6618)
* plugin/rewrite: Add "revert" parameter for EDNS0 options (https://github.com/coredns/coredns/pull/6893)
* container: Restored backwards compatibility of Current Workdir (https://github.com/coredns/coredns/pull/6731)
* plugin/auto: call OnShutdown() for each zone at its own OnShutdown() (https://github.com/coredns/coredns/pull/6705)
* plugin/dnstap: log queue and buffer memory size configuration (https://github.com/coredns/coredns/pull/6591)
* plugin/bind: add zone for link-local IPv6 instead of skipping (https://github.com/coredns/coredns/pull/6547)
* plugin/kubernetes: only create PTR records for endpoints with hostname defined (https://github.com/coredns/coredns/pull/6898)
* plugin/rewrite: execute the reversion in reversed order (https://github.com/coredns/coredns/pull/6872)
* plugin/etcd: fix etcd connection leakage during reload (https://github.com/coredns/coredns/pull/6646)
* plugin/kubernetes: Add useragent (https://github.com/coredns/coredns/pull/6484)
* plugin/hosts: add hostsfile as label for coredns_hosts_entries (https://github.com/coredns/coredns/pull/6801)
* plugin/file: Fix zone parser error handling (https://github.com/coredns/coredns/pull/6680)
* plugin/forward: Add alternate option to forward plugin (https://github.com/coredns/coredns/pull/6681)
* plugin/file: return error when parsing the file fails (https://github.com/coredns/coredns/pull/6699)
* build: Generate zplugin.go correctly with third-party plugins (https://github.com/coredns/coredns/pull/6692)
