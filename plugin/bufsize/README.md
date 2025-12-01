# bufsize
## Name
*bufsize* - limits EDNS0 buffer size to prevent IP fragmentation.

## Description
*bufsize* limits a requester's UDP payload size to within a maximum value.
If a request with an OPT RR has a bufsize greater than the limit, the bufsize
of the request will be reduced. Otherwise the request is unaffected.
It prevents IP fragmentation, mitigating certain DNS vulnerabilities.
It cannot increase UDP size requested by the client, it can be reduced only.
This will only affect queries that have
an OPT RR ([EDNS(0)](https://www.rfc-editor.org/rfc/rfc6891)).

## Syntax
```txt
bufsize [SIZE]
```

**[SIZE]** is an int value for setting the buffer size.
The default value is 1232, and the value must be within 512 - 4096.
Only one argument is acceptable, and it covers both IPv4 and IPv6.

## Examples
Enable limiting the buffer size of outgoing query to the resolver (172.31.0.10):
```corefile
. {
    bufsize 1100
    forward . 172.31.0.10
    log
}
```

Enable limiting the buffer size as an authoritative nameserver:
```corefile
. {
    bufsize 1220
    file db.example.org
    log
}
```

## Considerations
- Setting 1232 bytes to bufsize may avoid fragmentation on the majority of networks in use today, but it depends on the MTU of the physical network links.
