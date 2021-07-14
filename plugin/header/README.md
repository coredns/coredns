# header

## Name

*header* - modifies the header for all the responses.

## Description

It ensures that the flags are in the desired state for all the responses. The modifications are made transparently for
the client.

## Syntax

~~~
header {
    ACTION FLAGS...
    ACTION FLAGS...
}
~~~

* **ACTION** defines the state for dns flags. Actions are evaluated in the order they are defined so last one has the
  most precedence. Allowed values are:
    * `set`
    * `clear`
* **FLAGS** are the dns flags that will be modified. Current supported flags include:
    * `aa` - Authoritative
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

Make sure recursive available `ra` and authoritative `aa` flags are set and recursive desired is cleared in all the
responses:

~~~ corefile
. {
    header {
        set ra aa 
        clear rd
    }
}
~~~
