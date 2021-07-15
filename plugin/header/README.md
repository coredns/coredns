# header

## Name

*header* - modifies the header for responses.

## Description

*header* ensures that the flags are in the desired state for responses. The modifications are made transparently for
the client.

## Syntax

~~~
header {
    ACTION FLAGS...
    ACTION FLAGS...
}
~~~

* **ACTION** defines the state for DNS message header flags. Actions are evaluated in the order they are defined so last one has the
  most precedence. Allowed values are:
    * `set`
    * `clear`
* **FLAGS** are the DNS header flags that will be modified. Current supported flags include:
    * `aa` - Authoritative(Answer)
    * `ra` - RecursionAvailable
    * `rd` - RecursionDesired

## Examples

Make sure recursive available `ra` flag is set in all the responses:

~~~ corefile
. {
    header {
        set ra
    }
}
~~~

Make sure "recursion available" `ra` and "authoritative answer" `aa` flags are set and "recursion desired" is cleared in all responses:

~~~ corefile
. {
    header {
        set ra aa
        clear rd
    }
}
~~~
