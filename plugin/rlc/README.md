# rlc

## Name

*rlc* - add a reverse lookup cache to the server.

## Description

The `rlc` plugin implements a reverse lookup cache to resolve IPs (PTR request) as they have been resolved before by A (AAA) and CNAME queries.

This is useful in scenarios where you need to resolve IP addresses in logging and monitoring to the names actually used during the request. Many CDNs resolve IP addresses to generic names (e.g., 1.2.3.4-cdn.net) or not at all. Also, internal IP addresses may not have PTR entries at all. The names for IP addresses are returned sorted in order of the last query result.

The plugin uses groupcache to distribute results across multiple instances. If run inside Kubernetes, peer detection can be done automatically.

There is a major caveat, though: The answers are synthetic and may not be consistent in all cases. After an entry is expired, a PTR request may not even return a result at all anymore.

This plugin can only be used once per server block.

## Syntax

~~~ txt
rlc {
    ttl [TTL] 
    capacity [CAPACITY] 
    
    groupcache [CACHE_PORT] [PEER_LIST]

    remote [REMOTE_INGEST_PORT]
}
~~~

**TTL** time to keep resolve addresses in memory in seconds
If **TTL** is not given, it defaults to 3600


**CAPACITY** max memory used by the cache
If **CAPACITY** is not given, it defaults to 2048

*remote* allows to ingest reverse lookup data from external sources via _dnstap_. The _dnstap_ must contain answers to be usefull.
**REMOTE_INGEST_PORT** port for remote ingestion  

**CACHE_PORT** tcp port for the groupcache to share cached data actoss multiple instances
If **CACHE_PORT** is not given, it defaults to 8000

**PEER_LIST** comma separated list of peers. If it is set to `k8s`, kubernetes autodiscovery is used.

## Examples

Enable rlc:

~~~ corefile
. {
    rlc {
        ttl 600 
        capacity 5000
        remote 8053
        groupcache 8000
    }
    cache 30
    forward . /etc/resolv.conf
    loop
    errors {
        consolidate 5m ".* i/o timeout$" warning
        consolidate 30s "^Failed to .+"
    }
    log
    debug
    prometheus :9153
    reload
    health
    ready
}
~~~



~~~ sh
%  dig +short login.microsoft.com
a.privatelink.msidentity.com.
prda.aadg.msidentity.com.
www.tm.a.prd.aadg.akadns.net.
20.190.160.22
40.126.32.76
40.126.32.133
20.190.160.17
20.190.160.20
40.126.32.136
40.126.32.72
40.126.32.138

%  dig +short -x 40.126.32.138
login.microsoft.com.
www.tm.a.prd.aadg.akadns.net.
prda.aadg.msidentity.com.
a.privatelink.msidentity.com.
~~~

## See Also
 