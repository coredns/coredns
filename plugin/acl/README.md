# acl

*acl* - enforces basic access control policies and prevents unauthorized access to protected servers.

## Description

With `acl` enabled, users are able to filter DNS queries with filtering rules, i.e. allowing authorized queries to recurse or blocking unauthorized queries towards protected DNS zones.

This plugin can be used multiple times per Server Block.

## Syntax

```
acl [ZONES…] {
    ACTION type QTYPE net SOURCE
    ACTION type QTYPE file LOCAL_FILE
}
```

- **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block are used.
- **ACTION** (*allow* or *block*) defines the way to deal with DNS queries matched by this rule. The default action is *allow*, which means a DNS query not matched by any rules will be allowed to recurse.
- **QTYPE** is the query type to match for the requests to be allowed or blocked. Common resource record types are supported. *ANY* stands for all kinds of DNS queries.
- **SOURCE** is the source ip to match for the requests to be allowed or blocked. Typical CIDR notation and single IP address are supported. *ANY* stands for all possible source IP addresses.
- **LOCAL_FILE** is a local file including a list of IP addresses or subnets. It allows users to import blacklist/whitelist from third party directly without setting rules by hand.

## Examples

To demonstrate the usage of plugin acl, here we provide some typical examples.

Block all DNS queries with record type A from 192.168.0.0/16：
```
. {
    acl {
        block type A net 192.168.0.0/16
    }
}
```

Block all DNS queries from 192.168.0.0/16 except for 192.168.1.0/24:

```
. {
    acl {
        allow type ANY net 192.168.1.0/24
        block type ANY net 192.168.0.0/16
    }
}
```

Allow only DNS queries from 192.168.0.0/16:

```
. {
    acl {
        allow type ANY net 192.168.0.0/16
        block type ANY net ANY
    }
}
```

Block all DNS queries from 192.168.1.0/24 towards a.example.org:

```
example.org {
    acl a.example.org {
        block type ANY net 192.168.1.0/24
    }
}
```

Allow only DNS queries from private networks (`192.168.0.0/16`, `172.16.0.0/12`, `10.0.0.0/8`):
```
. {
    acl {
        allow type ANY net PRIVATE
        block type ANY net ANY
    }
}
```

Allow only DNS queries from local area network:
```
. {
    acl {
        allow type ANY net LOCAL
        block type ANY net ALL
    }
}
```

Block DNS queries from source ip on the local blacklist.
```
. {
    acl {
        block type ANY file /path/to/blacklist.txt
    }
}
```