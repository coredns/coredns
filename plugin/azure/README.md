# azure

## Name

*azure* - enables serving zone data from Microsoft Azure DNS service.

## Description

The azure plugin is useful for serving zones from resource record
sets in Microsoft Azure DNS. The `azure` plugin can be used when coredns is deployed on Azure or elsewhere.
This plugin supports all the DNS records supported by Azure, viz. A, AAAA, CAA, CNAME, MX, NS, PTR, SOA, SRV, and TXT record types.

## Syntax

~~~ txt
azure resource-group:dns-zone {
    tenant_id tenant_id
    client_id client_id
    client_secret client_secret
    subscription_id subscription_id
}
~~~

*   **`resource-group`** The resource group to which the dns hosted zones belong on Azure

*   **`dns-zone`** the dns zone that contains the resource record sets to be
    accessed.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **ZONES** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then
    only queries for those zones will be subject to fallthrough.

*   **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block

*   `environment` the azure environment to use. Defaults to `AzurePublicCloud`. Possible values: `AzureChinaCloud`, `AzureGermanCloud`, `AzurePublicCloud`, `AzureUSGovernmentCloud`.

## Examples

Enable `azure` with Azure credentials and fallthrough:

~~~ txt
. {
    azure resource-group-foo:foo.com {
      tenant_id 123abc-123abc-123abc-123abc
      client_id 123abc-123abc-123abc-123abc
      client_secret 123abc-123abc-123abc-123abc
      subscription_id 123abc-123abc-123abc-123abc
      fallthrough example.gov.
    }
}
~~~
