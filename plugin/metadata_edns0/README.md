# Name 
  
Plugin *metadata_edns0* is used for decoding EDNS0 related information from the DNS query and publish it as metadata.


# Description

~~~
metadata_edns0 {
      client_id local 0xffed
      group_id local 0xffee hex 16 0 16
      <label> local <id>
      <label> local <id> <encoded-format> <params of format ...>
}
~~~

So far, only 'hex' format is supported with params <length> <start> <end>


