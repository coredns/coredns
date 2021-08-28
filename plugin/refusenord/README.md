# refusenord

## Name

*refusenord* - enforces that all queries without RD (recursion desired) bit are answered with REFUSED. This prevents DNS cache snooping.

## Description

With `refusenord` enabled, users are able to prevent DNS cache snooping by refusing all DNS queries without RD bit set

## Syntax

```
refusenord
```

## Examples

Block all DNS queries without RD bit

~~~ corefile
. {
    refusenord
}
~~~
