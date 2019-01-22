# Name 
  
Plugin *metadata_edns0* is used for decoding EDNS0 related information from the DNS query and publish it as metadata.


# Description

~~~
metadata_edns0 {
      client_id 0xffed address
      group_id 0xffee hex 16 0 16
      <label> <id> <encoded-format> <params of format ...>
}
~~~

So far, only 'hex' format is supported with params <length> <start> <end>


