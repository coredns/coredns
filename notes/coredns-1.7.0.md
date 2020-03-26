+++
title = "CoreDNS-1.7.0 Release"
description = "CoreDNS-1.7.0 Release Notes."
tags = ["Release", "1.7.0", "Notes"]
release = "1.7.0"
date = 2020-03-24T10:00:00+00:00
author = "coredns"
+++

The CoreDNS team has released
[CoreDNS-1.7.0](https://github.com/coredns/coredns/releases/tag/v1.7.0).

This is a **backwards incompatible release**. Major changes include:
* Better metrics names (PR #3776)
* New `transfer` plugin that removes the need for plugins to perform their own zone transfers.
added `transfer` plugin that removes the need for plugins to perform their own zone transfers.

As this was already backwards incompatible release, we took the liberty to stuff is much of it in
one release as possible to minimize the disruption going forward.

## Brought to You By
## Noteworthy Changes
