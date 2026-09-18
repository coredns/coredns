# azure

## Name

*azure* - enables serving zone data from Microsoft Azure DNS service.

## Description

The azure plugin is useful for serving zones from Microsoft Azure DNS. The *azure* plugin supports
all the DNS records supported by Azure, viz. A, AAAA, CNAME, MX, NS, PTR, SOA, SRV, and TXT
record types. NS record type is not supported by azure private DNS.

Zone data is loaded asynchronously after startup and refreshed every minute.
An unavailable zone or zone-listing error is logged without preventing CoreDNS from
starting or other configured zones from being updated. Each zone listing,
including retries and pagination, has a one-minute timeout.
Configuration and credential initialization errors still prevent startup.

Until a zone has been successfully loaded, queries for it return SERVFAIL unless
`fallthrough` is explicitly configured. Only complete, successful updates replace
the in-memory zone. Failed updates, including a deleted Azure zone returning an
error, retain the last successfully loaded data and are retried. Remove the zone
from the Corefile to stop serving this retained data. Snapshots are not persisted
across restarts or configuration reloads.

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

*   **CLIENT_ID** and **CLIENT_SECRET** are the credentials for Azure, and `tenant` specifies the
    **TENANT_ID** to be used. **SUBSCRIPTION_ID** is the subscription ID. All of these are needed
    to access the data in Azure.

*  `environment` specifies the Azure **ENVIRONMENT**.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **ZONES** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative.

*   `access`  specifies if the zone is `public` or `private`. Default is `public`.

## Examples

Enable the *azure* plugin with Azure credentials for private zones `example.org`, `example.private`:

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

## See Also

The [Azure DNS Overview](https://docs.microsoft.com/en-us/azure/dns/dns-overview).
