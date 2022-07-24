+++
title = "CoreDNS-1.8.6 Release"
description = "CoreDNS-1.8.6 Release Notes."
tags = ["Release", "1.8.6", "Notes"]
release = "1.8.6"
date = "2021-10-07T00:00:00+00:00"
author = "coredns"
+++

This is a small bug fix release.

## Brought to You By

Chris O'Haver,
Miek Gieben.

## Noteworthy Changes

* plugin/kubernetes: fix reload panic (https://github.com/coredns/coredns/pull/4881)
* plugin/kubernetes: Don't use pod names longer than 63 characters as dns labels (https://github.com/coredns/coredns/pull/4908)
