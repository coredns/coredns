# azureDNS

## Name

*azureDNS* - enables serving zone data from Microsoft Azure DNS service.

## Description

The azureDNS plugin is useful for serving zones from resource record
sets in Microsoft Azure DNS. The `azureDNS` plugin can be used when coredns is deployed on Azure or elsewhere.

## Syntax

~~~ txt
azureDNS [resource-group:dns-zone] {
    [azure_auth_location <path to azure_auth_location>]
    [azure_tenant_id <azure_tenant_id>]
    [azure_client_id <azure_client_id>]
    [azure_client_secret <azure_client_secret>]
    [azure_subscription_id <azure_subscription_id>]
    upstream
    fallthrough [ZONES...]
}
~~~

*   **`resource-group`** The resource group to which the dns hosted zones belong on Azure

*   **`dns-zone`** the dns zone that contains the resource record sets to be
    accessed.

*   **`azure_auth_location`** the path to the Azure CLI authentication file. You can also provide the credentials individually (`azure_tenant_id`, `azure_client_id`, `azure_client_secret`, `azure_subscription_id`)

*   `upstream`is used for resolving services that point to external hosts (eg. used to resolve
    CNAMEs). CoreDNS will resolve against itself.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **[ZONES...]** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then
    only queries for those zones will be subject to fallthrough.

*   **ZONES** zones it should be authoritative for. If empty, the zones from the configuration block

## Examples

Enable azureDNS with Azure CLI authentication file and an upstream:

~~~ txt
. {
    azureDNS resource-group-foo:foo.com {
      azure_auth_location /tmp/config.json
    }
    forward . 10.0.0.1
}
~~~

Format for Azure CLI authentication file:
~~~ txt
{
  "clientId": "123abc-123abc-123abc-123abc",
  "clientSecret": "123abc-123abc-123abc-123abc",
  "subscriptionId": "123abc-123abc-123abc-123abc",
  "tenantId": "123abc-123abc-123abc-123abc"
}

~~~
Enable route53 with explicit Azure credentials:

~~~ txt
. {
    azureDNS resource-group-foo:foo.com {
      azure_tenant_id 123abc-123abc-123abc-123abc
      azure_client_id 123abc-123abc-123abc-123abc
      azure_client_secret 123abc-123abc-123abc-123abc
      azure_subscription_id 123abc-123abc-123abc-123abc
    }
}
~~~

Enable azureDNS with fallthrough:

~~~ txt
. {
    azureDNS resource-group-foo:foo.com {
      fallthrough example.gov.
    }
}
~~~
