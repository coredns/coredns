# loadbalance

## Name

*loadbalance* - randomizes the order of A, AAAA and MX records  and optionally prefers specific subnets.

## Description

The *loadbalance* will act as a round-robin DNS load balancer by randomizing the order of A, AAAA,
and MX records in the answer.

See [Wikipedia](https://en.wikipedia.org/wiki/Round-robin_DNS) about the pros and cons of this
setup. It will take care to sort any CNAMEs before any address records, because some stub resolver
implementations (like glibc) are particular about that.

## Syntax

~~~
loadbalance [round_robin | weighted WEIGHTFILE] {
			reload DURATION
			prefer CIDR [CIDR...]
			limit_answers NUM_ANSWERS
}
~~~
* `round_robin` policy randomizes the order of  A, AAAA, and MX records applying a uniform probability distribution. This is the default load balancing policy.

* `weighted` policy assigns weight values to IPs to control the relative likelihood of particular IPs to be returned as the first
(top) A/AAAA record in the answer. Note that it does not shuffle all the records in the answer, it is only concerned about the first A/AAAA record
returned in the answer.

Additionally, the plugin supports subnet-based ordering using the `prefer` directive, which reorders A/AAAA records so that IPs from preferred subnets appear first.

 * **WEIGHTFILE** is the file containing the weight values assigned to IPs for various domain names. If the path is relative, the path from the **root** plugin will be prepended to it. The format is explained below in the *Weightfile* section.

 * **DURATION** interval to reload `WEIGHTFILE` and update weight assignments if there are changes in the file. The default value is `30s`. A value of `0s` means to not scan for changes and reload.

 * `limit_answers` keeps at most **NUM_ANSWERS** records from each A, AAAA, or MX
   RRset in the answer section. **NUM_ANSWERS** must be a positive integer. The
   default is no limit. Selection happens after the configured policy and any
   `prefer` ordering, keeping the first records of each RRset (same name, class,
   and type). With `weighted`, only the first address is selected by weight;
   the remaining addresses retain their existing order.

   CNAME/DNAME records and other record types do not count toward the limit.
   Authority and additional sections are not limited. A response can therefore
   contain more than **NUM_ANSWERS** records in total.

   This is an opt-in local answer rewriting policy for unsigned data. It returns
   the selected subset without setting the TC flag. Downstream caches may cache
   that subset for its TTL; it does not balance individual connections. Queries
   with the DNSSEC OK (DO) bit, responses with the authenticated data (AD) or TC
   bit, responses containing signatures (RRSIG, SIG, or TSIG), and zone transfers
   bypass limiting. These exceptions preserve signed RRsets and incomplete
   responses. The configured ordering still applies as usual.


## Weightfile

The generic weight file syntax:

~~~
# Comment lines are ignored

domain-name1
ip11 weight11
ip12 weight12
ip13 weight13

domain-name2
ip21 weight21
ip22 weight22
# ... etc.
~~~

where `ipXY` is an IP address for `domain-nameX` and `weightXY` is the weight value associated with that IP. The weight values are in the range of [1,255].

The `weighted` policy selects one of the address record in the result list and moves it to the top (first) position in the list. The random selection takes into account the weight values assigned to the addresses in the weight file. If an address in the result list is associated with no weight value in the weight file then the default weight value "1" is assumed for it when the selection is performed.


## Examples

Load balance replies coming back from Google Public DNS:

~~~ corefile
. {
    loadbalance round_robin
    forward . 8.8.8.8 8.8.4.4
}
~~~

Use the `weighted` strategy to load balance replies supplied by the **file** plugin. We assign weight vales `3`, `1` and `2` to the IPs `100.64.1.1`, `100.64.1.2` and `100.64.1.3`, respectively. These IPs are addresses in A records for the domain name `www.example.com` defined in the `./db.example.com` zone file. The ratio between the number of answers in which `100.64.1.1`, `100.64.1.2` or `100.64.1.3` is in the top (first) A record should converge to  `3 : 1 : 2`.  (E.g. there should be twice as many answers with `100.64.1.3` in the top A record than with `100.64.1.2`).
Corefile:

~~~ corefile
example.com {
        file ./db.example.com {
                reload 10s
        }
        loadbalance weighted ./db.example.com.weights {
                    reload 10s
        }
}
~~~

weight file `./db.example.com.weights`:

~~~
www.example.com
100.64.1.1 3
100.64.1.2 1
100.64.1.3 2
~~~

### Subnet Prioritization

Prioritize IPs from 10.9.20.0/24 and 192.168.1.0/24:

```corefile
. {
    loadbalance round_robin {
        prefer 10.9.20.0/24 192.168.1.0/24
    }
    forward . 1.1.1.1
}
```

If the DNS response includes multiple A/AAAA records, the plugin will reorder them to place the ones matching preferred subnets first.

### Limit Answers

Select one address from each answer RRset after round-robin ordering:

~~~ corefile
example.com {
    loadbalance {
        limit_answers 1
    }
    file db.example.com
}
~~~

An unsigned A reply containing two CNAME records and five addresses will retain
both CNAME records and one address. The same option can be combined with
`weighted`, `reload`, and `prefer` in a single block. If `prefer` is configured,
preferred addresses are selected before other addresses.
