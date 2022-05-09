# RoundRobin

## Name
*roundrobin* - plugin provides several round-robin strategies that shuffle A and AAAA 
records in a DNS response.

## Description
The roundrobin plugin implements [round-robin](https://en.wikipedia.org/wiki/Round-robin_scheduling)
strategies on returned A and AAAA records. 
```
roundrobin [STRATEGY]
```
`[STRATEGY]` defines how A and AAAA will be shuffled. If there are records other than A or AAAA in the
answer, they are moved to the end of the answer section in the exact order as they were in the answer 
section before the roundrobin applied. 
- [stateful](#stateful)
- [stateless](#stateless)
- [random](#random)

## Syntax
~~~
roundrobin [STRATEGY]
~~~
For each zone that you want to shake for; the default strategy is stateful.

## Examples
### stateful
Stateful will probably be the strategy you expect from a round-robin. Each query rotates the A or AAAA records 
by one position. The RoundRobin plugin remembers the positions of the last query for ten minutes and manages changes 
to the answers: clears non-existing records, adds new ones, shuffling (rotation by one position) and garbage collection.

_NOTE: key to the request state is the pair `EDNS0_SUBNET` and request `domain name`, [see more](https://en.wikipedia.org/wiki/EDNS_Client_Subnet)._

#### Example
~~~ corefile
.:5053 {
    log
    roundrobin stateful
}
~~~
After you run the `dig` command repeatedly, the records for the `myhosts.com` domain rotate. The roundrobin plugin can 
handle the number of records or individual records changing. In addition, garbage collector clears the records every ten 
minutes of inactivity.

```shell
# runnig against a local hosts plugin `hosts etchosts` 

dig @localhost -p 5053 myhost.com                                 dig @localhost -p 5053 myhost.com                        
myhost.com.             3600    IN      A       200.0.0.2         myhost.com.             3600    IN      A       200.0.0.3
myhost.com.             3600    IN      A       200.0.0.3         myhost.com.             3600    IN      A       200.0.0.4
myhost.com.             3600    IN      A       200.0.0.4         myhost.com.             3600    IN      A       200.0.0.1
myhost.com.             3600    IN      A       200.0.0.1         myhost.com.             3600    IN      A       200.0.0.2

dig @localhost -p 5053 myhost.com                                 dig @localhost -p 5053 myhost.com                        
myhost.com.             3600    IN      A       200.0.0.4         myhost.com.             3600    IN      A       200.0.0.1
myhost.com.             3600    IN      A       200.0.0.1         myhost.com.             3600    IN      A       200.0.0.2
myhost.com.             3600    IN      A       200.0.0.2         myhost.com.             3600    IN      A       200.0.0.3
myhost.com.             3600    IN      A       200.0.0.3         myhost.com.             3600    IN      A       200.0.0.4
```

### stateless
Stateless is useful where you require extremely high scalability, customization, or you cannot use stateful. The state 
is stored on the client and like in HTTP, you send the records back to CoreDNS in the DNS query having the section Extra 
records filled. ( `EDNS0_LOCAL`, see GO example below). The `stateless` plugin takes care of shuffling, 
clears non-existing records and adds new ones. As in HTTP, the client must store the response in its memory for the next 
request. Sending client data must be in form of text encoded as a byte-array where `_rr_state=` is a constant 
identifying the state followed by the json containing the client state, see: `_rr_state={"ip":["10.0.0.1","10.2.2.1","10.1.1.2"]}` 

#### Example
~~~ corefile
.:5053 {
    log
    roundrobin stateless
}
~~~
The state must be managed on the client side. The following example creates a simple DNS query that can be consumed by 
the stateless plugin.
```go
type State struct {
    IPs    []string `json:"ip"`
}

func statelessExchange(state State) (r *dns.Msg, err error){
    json, _ := json.Marshal(state)
    opt := new(dns.EDNS0_LOCAL)
    opt.Data = append([]byte("_rr_state="),json...)
    ext := new(dns.OPT)
    ext.Hdr.Name = "."
    ext.Hdr.Rrtype = dns.TypeOPT
    ext.Option = append(ext.Option, opt)
    msg := new(dns.Msg)
    msg.SetQuestion("myhost.com.", dns.TypeA)
    msg.Extra = append(msg.Extra, ext)
    return dns.Exchange(msg, fmt.Sprintf("%s:%v", dnsServer, port))
}
```

```
# runnig against a local hosts plugin `hosts etchosts` 

_rr_state={"ip":[]}                                                 _rr_state={"ip":["200.0.0.2","200.0.0.3","200.0.0.4","200.0.0.1"]}
myhost.com.   3600    IN      A       200.0.0.2                       myhost.com.   3600    IN      A       200.0.0.3          
myhost.com.   3600    IN      A       200.0.0.3                       myhost.com.   3600    IN      A       200.0.0.4          
myhost.com.   3600    IN      A       200.0.0.4                       myhost.com.   3600    IN      A       200.0.0.1          
myhost.com.   3600    IN      A       200.0.0.1                       myhost.com.   3600    IN      A       200.0.0.2          

_rr_state={"ip":["200.0.0.3","200.0.0.4","200.0.0.1","200.0.0.2"]}    _rr_state={"ip":["200.0.0.4","200.0.0.1","200.0.0.2","200.0.0.3"]}
myhost.com.   3600    IN      A       200.0.0.4                       myhost.com.   3600    IN      A       200.0.0.1          
myhost.com.   3600    IN      A       200.0.0.1                       myhost.com.   3600    IN      A       200.0.0.2          
myhost.com.   3600    IN      A       200.0.0.2                       myhost.com.   3600    IN      A       200.0.0.3          
myhost.com.   3600    IN      A       200.0.0.3                       myhost.com.   3600    IN      A       200.0.0.4          
```

### random
~~~ corefile
.:5053 {
    log
    roundrobin random
}
~~~
Random is the simplest strategy available. It is simple, fast, and does not work with any state. On the other hand, 
it does not offer consistent results. Responses may be repeated, and you have no guarantee that the returned IP 
addresses will be provided equally. 

#### Example
It is clear from the following example that if you work with the first address in the list, for example, you can easily 
select the same address (200.0.0.1) three times before something else comes up.
```shell
# runnig against a local hosts plugin `hosts etchosts` 

dig @localhost -p 5053 myhost.com                                 dig @localhost -p 5053 myhost.com                        
myhost.com.             3600    IN      A       200.0.0.1         myhost.com.             3600    IN      A       200.0.0.1
myhost.com.             3600    IN      A       200.0.0.2         myhost.com.             3600    IN      A       200.0.0.2
myhost.com.             3600    IN      A       200.0.0.3         myhost.com.             3600    IN      A       200.0.0.4
myhost.com.             3600    IN      A       200.0.0.4         myhost.com.             3600    IN      A       200.0.0.3

dig @localhost -p 5053 myhost.com                                 dig @localhost -p 5053 myhost.com                        
myhost.com.             3600    IN      A       200.0.0.1         myhost.com.             3600    IN      A       200.0.0.3
myhost.com.             3600    IN      A       200.0.0.3         myhost.com.             3600    IN      A       200.0.0.1
myhost.com.             3600    IN      A       200.0.0.2         myhost.com.             3600    IN      A       200.0.0.4
myhost.com.             3600    IN      A       200.0.0.4         myhost.com.             3600    IN      A       200.0.0.2
```