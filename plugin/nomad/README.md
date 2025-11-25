# nomad

## Name

*nomad* - enables reading zone data from a Nomad cluster.

## Description

This plugin serves DNS records for services registered with Nomad. Nomad 1.3+ comes with [support for discovering services](https://www.hashicorp.com/en/blog/nomad-service-discovery) with an in-built service catalogue that is available via the HTTP API. This plugin extends the HTTP API and provides a DNS interface for querying the service catalogue.

The query can be looked up with the format `[service].[namespace].service.nomad`. The plugin currently handles A, AAAA and SRV records. Refer to [#Usage Example](#usage-example) for more details.

## Example job template

```
job "dns" {
  type = "service"

  group "dns" {
    network {
      port "dns" {
        static = 1053
      }
    }
    task "dns" {
      driver = "docker"

      config {
        image = "coredns/coredns:latest"
        ports = ["dns"]
        args = ["-conf", "/secrets/coredns/Corefile", "-dns.port", "1053"]
      }

      service {
        name         = "hostmaster"
        provider     = "nomad"
        port         = "dns"
        address_mode = "driver"
      }

      identity {
        env = true
      }

      template {
        data          = <<EOF
. {
  forward . 1.1.1.1
}

service.nomad. {
  errors
  debug
  health
  log
  nomad service.nomad {
    address unix:///secrets/api.sock
    ttl 10
  }
  cache 30
}
EOF
        destination   = "secrets/coredns/Corefile"
        change_mode   = "signal"
        change_signal = "SIGHUP"
      }
    }
  }
}
```

## Syntax

~~~ txt
nomad [ZONE] {
~~~

With only the plugin specified, the *nomad* plugin will default to `service.nomad` zone.

~~~ txt
nomad [ZONE] {
    address URL
    filter FILTER
    token TOKEN
    ttl DURATION
}
~~~

* `address` The address where a Nomad agent (server) is available. **URL** defaults to `http://127.0.0.1:4646`.

* `filter` allows you to filter Nomad services. **FILTER** defaults to `""`. Uses [filtering](https://developer.hashicorp.com/nomad/api-docs#filtering) syntax.

* `token` The SecretID of an ACL token to use to authenticate API requests with if the Nomad cluster has ACL enabled. **TOKEN** defaults to `""`.

* `ttl` allows you to set a custom TTL for responses. **DURATION** defaults to `30 seconds`. The minimum TTL allowed is `0` seconds, and the maximum is capped at `3600` seconds. Setting TTL to 0 will prevent records from being cached. The unit for the value is seconds.

## Metrics

If monitoring is enabled (via the *prometheus* directive) the following metric is exported:

* `coredns_nomad_success_requests_total{namespace,server}` - Counter of DNS requests handled successfully.
* `coredns_nomad_failed_requests_total{namespace,server}` - Counter of DNS requests failed.

The `server` label indicated which server handled the request. `namespace` indicates the namespace of the service in the query.

## Ready

This plugin reports readiness to the ready plugin. It will be ready only when it has successfully connected to the Nomad server. It queries the [`/v1/agent/self`](https://developer.hashicorp.com/nomad/api-docs/agent#query-self) endpoint to check if it is ready.

## Examples

Enable nomad with and resolve all services with `.nomad` as the suffix. `cache` plugin is used to cache the responses for 30 seconds. This avoids a lookup to the Nomad server for every request.

```
service.nomad.:1053 {
    log
    cache
    errors
    nomad service.nomad {
        address http://127.0.0.1:4646 http://127.0.0.2:4646 http://127.0.0.3:4646
        ttl 10
    }
    cache 30
}
```

## Authentication

`nomad` plugin uses a default Nomad configuration to create an API client. Options like the HTTP address and the token can be specified in Corefile. However, Nomad Go SDK can also additionally read these environment variables.

- `NOMAD_TOKEN`
- `NOMAD_ADDR`
- `NOMAD_REGION`
- `NOMAD_NAMESPACE`
- `NOMAD_HTTP_AUTH`
- `NOMAD_CACERT`
- `NOMAD_CAPATH`
- `NOMAD_CLIENT_CERT`
- `NOMAD_CLIENT_KEY`
- `NOMAD_TLS_SERVER_NAME`
- `NOMAD_SKIP_VERIFY`

You can read about them in detail [here](https://developer.hashicorp.com/nomad/docs/reference/runtime-environment-settings).

## Usage Example

### A record

```
dig redis.default.service.nomad @127.0.0.1 -p 1053

; <<>> DiG 9.18.1-1ubuntu1.2-Ubuntu <<>> redis.default.service.nomad @127.0.0.1 -p 1053
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 54986
;; flags: qr aa rd; QUERY: 1, ANSWER: 3, AUTHORITY: 0, ADDITIONAL: 1
;; WARNING: recursion requested but not available

;; OPT PSEUDOSECTION:
; EDNS: version: 0, flags:; udp: 1232
; COOKIE: bdc9237f49a1f744 (echoed)
;; QUESTION SECTION:
;redis.default.service.nomad.		IN	A

;; ANSWER SECTION:
redis.default.service.nomad.	10	IN	A	192.168.29.76
redis.default.service.nomad.	10	IN	A	192.168.29.76
redis.default.service.nomad.	10	IN	A	192.168.29.76

;; Query time: 4 msec
;; SERVER: 127.0.0.1#1053(127.0.0.1) (UDP)
;; WHEN: Thu Jan 05 12:12:25 IST 2023
;; MSG SIZE  rcvd: 165
```

### SRV Record

Since an A record doesn't contain the port number, SRV record can be used to query the port number of a service.

```
dig redis.default.service.nomad @127.0.0.1 -p 1053 SRV

; <<>> DiG 9.18.1-1ubuntu1.2-Ubuntu <<>> redis.default.service.nomad @127.0.0.1 -p 1053 SRV
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NOERROR, id: 49945
;; flags: qr aa rd; QUERY: 1, ANSWER: 3, AUTHORITY: 0, ADDITIONAL: 4
;; WARNING: recursion requested but not available

;; OPT PSEUDOSECTION:
; EDNS: version: 0, flags:; udp: 1232
; COOKIE: 14572535f3ba6648 (echoed)
;; QUESTION SECTION:
;redis.default.service.nomad.		IN	SRV

;; ANSWER SECTION:
redis.default.service.nomad.	8	IN	SRV	10 10 25395 redis.default.service.nomad.
redis.default.service.nomad.	8	IN	SRV	10 10 20888 redis.default.service.nomad.
redis.default.service.nomad.	8	IN	SRV	10 10 26292 redis.default.service.nomad.

;; ADDITIONAL SECTION:
redis.default.service.nomad.	8	IN	A	192.168.29.76
redis.default.service.nomad.	8	IN	A	192.168.29.76
redis.default.service.nomad.	8	IN	A	192.168.29.76

;; Query time: 0 msec
;; SERVER: 127.0.0.1#1053(127.0.0.1) (UDP)
;; WHEN: Thu Jan 05 12:12:20 IST 2023
;; MSG SIZE  rcvd: 339
```

### SOA Record

```
$ dig @localhost -p 1053 1dns.default.service.nomad.

; <<>> DiG 9.18.12-0ubuntu0.22.04.2-Ubuntu <<>> @localhost -p 1053 1dns.default.service.nomad.
; (1 server found)
;; global options: +cmd
;; Got answer:
;; ->>HEADER<<- opcode: QUERY, status: NXDOMAIN, id: 21012
;; flags: qr aa rd; QUERY: 1, ANSWER: 1, AUTHORITY: 0, ADDITIONAL: 1
;; WARNING: recursion requested but not available

;; OPT PSEUDOSECTION:
; EDNS: version: 0, flags:; udp: 1232
; COOKIE: 6d146bb140b4d8ca (echoed)
;; QUESTION SECTION:
;1dns.default.service.nomad.    IN      A

;; ANSWER SECTION:
1dns.default.service.nomad. 5   IN      SOA     ns1.1dns.default.service.nomad. ns1.1dns.default.service.nomad. 1 3600 600 604800 3600

;; Query time: 0 msec
;; SERVER: 127.0.0.1#1053(localhost) (UDP)
;; WHEN: Wed Aug 23 21:14:41 EEST 2023
;; MSG SIZE  rcvd: 189
```
