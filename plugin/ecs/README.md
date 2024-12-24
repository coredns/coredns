# ecs_remap

Coredns plugin to pass the client's source address in the EDNS client subnet option (if not already present in the query), to return better results when forwarding queries to CDNs.

Usage:

```
. {
    ecs {
        mask_v4 SIZE
        mask_v6 SIZE
    }

    # Do not use 1.1.1.1, it explicitly blocks ECS
    forward . 8.8.8.8
}
```

Options:

* `mask_v4` - Optional (defaults to 24), specifies the mask to be applied to the client's source IPv4 address for privacy protection
* `mask_v6` - Optional (defaults to 56), specifies the mask to be applied to the client's source IPv6 address for privacy protection