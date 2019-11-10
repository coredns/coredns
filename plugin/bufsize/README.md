# bufsize
## Name
*bufsize* - sizes EDNS0 buffer size to prevent IP fragmentation.

## Description
*bufsize* limits a requester's UDP payload size.  
It prevents IP fragmentation so that to deal with DNS vulnerability.

## Syntax
```txt
bufsize [SIZE]
```

**[SIZE]** is an int value for setting the buffer size.  
The default value is 512, and the value must be within 512 - 4096.  
Only one argument is acceptable, and it covers both IPv4 and IPv6.

## Examples
### As a forwarder
```corefile
. {
    bufsize 512
    forward . 172.31.0.10
    log
}
```

If you run a resolver on 172.31.0.10, the buffer size of incoming query on the resolver will be set to 512 bytes.

### As an authoritative nameserver
```corefile
. {
    bufsize 512
    file db.example.org
    log
}
```

Also you can limit the buffer size as an authoritative nameserver.  
If the answer from *file* plugin exceeds 512 bytes, the response will be truncated.

## Considerations
For now, if a client does not use EDNS, this plugin adds OPT RR.