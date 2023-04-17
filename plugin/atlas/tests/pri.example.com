$TTL        3600
@       IN      SOA     ns1.example.com. info.example.com. (
                        2022061501       ; serial, todays date + todays serial #
                        10800            ; refresh, seconds
                        3600             ; retry, seconds
                        604800           ; expire, seconds
                        3600 )           ; minimum, seconds
;

example.com. 3600      A          1.2.3.4
www          3600      A          1.2.3.4
autoconfig   3600      CNAME      autoconfig.provider.com.
mail         3600      CNAME      mx.provider.com.
example.com. 3600      MX     10  mail.example.com.
example.com.           NS         ns1.example.com.
example.com.           NS         ns2.example.com.
example.com. 3600      TXT        "some text record"
