# minimal

## Name

*minimal* - minimizes the size of the DNS query response 

## Description

The minimal plugin controls the addition of records to the AUTHORITY and ADDITIONAL section of the dns query response.

## Syntax

~~~
minimal yes | no
~~~

## Examples

~~~ corefile
. {
    minimal yes
    forward . 8.8.8.8 
}
~~~

## Design and Implementation details

If minimal is set as yes, this plugin wraps a response writer around dns.ResponseWriter and passes it on the to the next plugin 
in the plugin chain. If minimal is explicitly set as no, then this plugin will not be added at all. This should be better than 
making this plugin a passthrough plugin

This response writer implements the dns.ResponseWriter interface. 

The writer first checks the type of the dns.Msg. If the response type is any type apart from NoError, we just return the response back to the 
client. We even return back any Delegations because delegations sometimes contain glue records which are essential for dns resolution.

If the response type is NoError, then we strip away the ADDITIONAL and AUTHORITY sections away and ensure that only the answer section is
present in the dns response which is essential to complete the dns query succesfully.

Pseudo code:

type MinimalResponseWriter struct {
  dns.ResponseWriter
}

func (m *MinimalResponseWriter) minimalMsg(res *dns.Msg) error {
  type := response.Typify(res)

  if type != response.NoError {
    m.ResponseWriter.WriteMsg(m)
  }

  // will such a case ever arise?????
  if len(res.Answer) == 0 {
    return m.ResponseWriter.WriteMsg(m)
  }

  // set the AUTHORITY and ADDITIONAL record to nil. Empty the records.
  res.Ns = nil
  res.Extra = nil

  return m.ResponseWriter.WriteMsg(m)
}

Tests: 