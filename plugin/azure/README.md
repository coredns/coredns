# azure

## Name

*azure* - enables serving zone data from Microsoft Azure DNS service.

## Description

The azure plugin is useful for serving zones from Microsoft Azure DNS. The *azure* plugin supports
all the DNS records supported by Azure, viz. A, AAAA, CNAME, MX, NS, PTR, SOA, SRV, and TXT
record types. NS record type is not supported by azure private DNS.

## Syntax

~~~ txt
azure RESOURCE_GROUP:ZONE... {
    tenant TENANT_ID
    client CLIENT_ID
    secret CLIENT_SECRET
    subscription SUBSCRIPTION_ID
    environment ENVIRONMENT
    fallthrough [ZONES...]
    access private
}
~~~

*   **RESOURCE_GROUP:ZONE** is the resource group to which the hosted zones belongs on Azure,
    and **ZONE** the zone that contains data.

*   **CLIENT_ID** is the client id (also known as the application id) of the Entra ID (formerly
    known as Azure AD) Service Principal or User Assigned Managed identity. If you are using
    a system assigned managed identity this should be omitted as the authentication package
    for Azure defaults to using the system assigned identity (otherwise you will receive an
    error stating the identity does not exist because it assumes you wish to use a user
    assigned managed identity).

*   **CLIENT_SECRET** associated with the Service Principal. The **CLIENT_SECRET**
    is required if you are using an Entra ID Service Principal. If you
    are using a system or user assigned managed identity then this should not be set.

*   The  `tenant` specifies the **TENANT_ID** to be used (mandatory). 

*   The **SUBSCRIPTION_ID** is the subscription ID (mandatory).

*  `environment` specifies the Azure **ENVIRONMENT**, defaults to AzurePublic.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **ZONES** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative.

*   `access`  specifies if the zone is `public` or `private`. Default is `public`.

## Examples

Enable the *azure* plugin with Azure service Principal credentials for private zones `example.org`, `example.private`:

~~~ txt
example.org {
    azure resource_group_foo:example.org resource_group_foo:example.private {
      tenant 123abc-123abc-123abc-123abc
      client 123abc-123abc-123abc-234xyz
      subscription 123abc-123abc-123abc-563abc
      secret mysecret
      access private
    }
}
~~~

Use a user assigned managed identity:

~~~ txt
example.org {
    azure resource_group_foo:example.org resource_group_foo:example.private {
      tenant 123abc-123abc-123abc-123abc
      client 123abc-123abc-123abc-234xyz
      subscription 123abc-123abc-123abc-563abc
      access private
    }
}
~~~

Use a system assigned managed identity (by omitting the client id):

~~~ txt
example.org {
    azure resource_group_foo:example.org resource_group_foo:example.private {
      tenant 123abc-123abc-123abc-123abc
      subscription 123abc-123abc-123abc-563abc
      access private
    }
}
~~~

## See Also

The [Azure DNS Overview](https://docs.microsoft.com/en-us/azure/dns/dns-overview).
