# bufsize
## Name
*bufsize* - modifies EDNS0 buffer size for protecting answers from vulnerability of IP flagmentation.

## Description
*bufsize* can limit requester's UDP payload size.  
It prevents fragmentation so that be ready for the DNS Flag Day 2020 and deal with DNS vulnerability.

## Syntax
```text
bufsize [SIZE]
```

**[SIZE]** is an int value for setting the buffer size.  
The default value is 512, and the value must be within 512 - 4096.  
Only one argument is acceptable

## Examples
```text
. {
    bufsize 512
    forward . 172.31.0.10
    log
}
```

If you run a resolver on 172.31.0.10, the bufsize of incoming query on the resolver will be set as 512 bytes.

## Motivation
In the next DNS Flag Day, message size should be considered to avoid IP fragmentation.  

https://dnsflagday.net/2020/
> Action: DNS Resolver Operators
> Requrirements on the resolver side are more or less the same as for authoritative: ensure that your servers can answer DNS queries over TCP (port 53), and configure an EDNS buffer size of 1232 bytes to avoid fragmentation. Remember to check your firewall(s) for problems with DNS over TCP!

We have to consider about fragment attack anyway, *bufsize* can limit requester's UDP payload size.  
This plugin can follow this draft too.  

https://tools.ietf.org/html/draft-fujiwara-dnsop-fragment-attack-01#section-4.3
> 4.3.  Limit requestor's UDP payload size to 512
>    Limiting EDNS0 requestor's UDP payload size [RFC6891] to 512 may be a
>    measure of path MTU attacks.

## Considerations
For now, if a client does not use EDNS, this plugin adds OPT RR.  
This behavior respect the following description on RFC 6891.

https://tools.ietf.org/html/rfc6891#section-6.1.1
> The OPT RR MAY be placed anywhere within the additional data section.