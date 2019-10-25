# bufsize
## Name
*bufsize* - modifies EDNS0 buffer size for protecting answers from vulnerability of IP flagmentation.

## Description
*bufsize* can limit requester's UDP payload size.  
It prevents IP fragmentation so that to be ready for the DNS Flag Day 2020 and to deal with DNS vulnerability.

## Syntax
```text
bufsize [SIZE]
```

**[SIZE]** is an int value for setting the buffer size.  
The default value is 512, and the value must be within 512 - 4096.  
Only one argument is acceptable.

## Examples
```text
. {
    bufsize 512
    forward . 172.31.0.10
    log
}
```

If you run a resolver on 172.31.0.10, the bufsize of incoming query on the resolver will be set as 512 bytes.

## Considerations
For now, if a client does not use EDNS, this plugin adds OPT RR.  
This behavior respect the following description on RFC 6891.

https://tools.ietf.org/html/rfc6891#section-6.1.1
> The OPT RR MAY be placed anywhere within the additional data section.