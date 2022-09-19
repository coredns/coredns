+++
title = "CoreDNS-1.9.4 Release"
description = "CoreDNS-1.9.4 Release Notes."
tags = ["Release", "1.9.4", "Notes"]
release = "1.9.4"
date = "2022-09-07T00:00:00+00:00"
author = "coredns"
+++

This is a release with many new features. The most notable addition is a new plugin tsig for validating
TSIG requests and signing responses. In header plugin a selector of `query` or `response` (default) is added for
applying the actions. This release also adds lots of enhancements and bug fixes.

## Brought to You By

Abirdcfly
Alex
AndreasHuber-CH
Andy Lindeman
Chris Narkiewicz
Chris O'Haver
Christoph Heer
Daniel Jolly
Konstantin Demin
Marius Kimmina
Md Sahil
Ondřej Benkovský
Shane Xie
TomasKohout
Vancl
Yong Tang


## Noteworthy Changes

* core: add log listeners for k8s_event plugin (https://github.com/coredns/coredns/pull/5451)
* core: log DoH HTTP server error logs in CoreDNS format (https://github.com/coredns/coredns/pull/5457)
* core: warn when domain names are not in RFC1035 preferred syntax (https://github.com/coredns/coredns/pull/5414)
* plugin/acl: add support for extended DNS errors (https://github.com/coredns/coredns/pull/5532)
* plugin/bufsize: do not expand query UDP buffer size if already set to a smaller value (https://github.com/coredns/coredns/pull/5602)
* plugin/cache: add cache disable option (https://github.com/coredns/coredns/pull/5540)
* plugin/cache: add metadata for wildcard record responses (https://github.com/coredns/coredns/pull/5308)
* plugin/cache: add option to adjust SERVFAIL response cache TTL (https://github.com/coredns/coredns/pull/5320)
* plugin/cache: correct responses to Authenticated Data requests (https://github.com/coredns/coredns/pull/5191)
* plugin/dnstap: add identity and version support for the dnstap plugin (https://github.com/coredns/coredns/pull/5555)
* plugin/file: add metadata for wildcard record responses (https://github.com/coredns/coredns/pull/5308)
* plugin/forward: enable multiple forward declarations (https://github.com/coredns/coredns/pull/5127)
* plugin/forward: health_check needs to normalize a specified domain name (https://github.com/coredns/coredns/pull/5543)
* plugin/forward: remove unused coredns_forward_sockets_open metric (https://github.com/coredns/coredns/pull/5431)
* plugin/header: add support for query modification (https://github.com/coredns/coredns/pull/5556)
* plugin/health: bypass proxy in self health check (https://github.com/coredns/coredns/pull/5401)
* plugin/health: don't go lameduck when reloading (https://github.com/coredns/coredns/pull/5472)
* plugin/k8s_external: add support for PTR requests (https://github.com/coredns/coredns/pull/5435)
* plugin/k8s_external: resolve headless services (https://github.com/coredns/coredns/pull/5505)
* plugin/kubernetes: make kubernetes client log in CoreDNS format (https://github.com/coredns/coredns/pull/5461)
* plugin/ready: reset list of readiness plugins on startup (https://github.com/coredns/coredns/pull/5492)
* plugin/rewrite: add PTR records to supported types (https://github.com/coredns/coredns/pull/5565)
* plugin/rewrite: fix a crash in rewrite plugin when rule type is missing (https://github.com/coredns/coredns/pull/5459)
* plugin/rewrite: fix out-of-index issue in rewrite plugin (https://github.com/coredns/coredns/pull/5462)
* plugin/rewrite: support min and max TTL values (https://github.com/coredns/coredns/pull/5508)
* plugin/trace : make zipkin HTTP reporter more configurable using Corefile (https://github.com/coredns/coredns/pull/5460)
* plugin/trace: read trace context info from headers for DOH (https://github.com/coredns/coredns/pull/5439)
* plugin/tsig: add new plugin TSIG for validating TSIG requests and signing responses (https://github.com/coredns/coredns/pull/4957)
