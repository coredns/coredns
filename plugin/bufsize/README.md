# bufsize
## Name
*bufsize* - modifying EDNS0 buffer size for the flag day and protecting by vulnerability of IP flagmentation.

## Description
In the next DNS Flag Day, message size should be considered to avoid IP fragmentation.  

https://dnsflagday.net/2020/  
> Until such a standard exists, we recommend that the EDNS buffer size should, by default, be set to a value small enough to avoid fragmentation on the majority of network links in use today.

## Syntax
```text
bufsize [SIZE]
```

**[SIZE]** is an int value for setting the buffer size.  
The default value is 512, and the value must be within 512 - 4096.  
Only one argument is acceptable.

## Examples
```text
example.org {
    bufsize 1232
}
```

## Considerations
For now, if a client does not use EDNS, this plugin would add OPT RR.  
This behavior respects the following description on RFC 6891.

https://tools.ietf.org/html/rfc6891#section-6.1.1
> The OPT RR MAY be placed anywhere within the additional data section.