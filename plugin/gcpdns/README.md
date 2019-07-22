# gcpdns

## Name

*gcpdns* - enables serving zone data from Google Cloud DNS.

## Description

The gcpdns plugin is useful for serving zones from resource record sets in
Google Cloud DNS. This plugin supports all Google Cloud DNS records
([https://cloud.google.com/dns/docs/overview](https://cloud.google.com/dns/docs/overview#supported_dns_record_types)).
The gcpdns plugin can be used when CoreDNS is deployed on Google Cloud or
elsewhere.

## Syntax

~~~ txt
gcpdns [DNS_NAME:GCP_PROJECT/ZONE_NAME...] {
    [gcp_service_account_json GCP_SA_ENV_VAR] | [gcp_service_account_file GCP_CREDENTIALS_FILE]
    fallthrough [ZONES...]
}
~~~

* **DNS_NAME** the name of the domain to be accessed. When there are multiple
  zones with overlapping domains (private vs. public hosted zone), CoreDNS does
  the lookup in the given order here. Therefore, for a non-existing resource
  record, SOA response will be from the rightmost zone.

* **GCP_PROJECT** the GCP project that hosts the GCP Cloud DNS zone.

* **ZONE_NAME** the name of the hosted zone that contains the resource record
  sets to be accessed.

* `gcp_service_account_json` **GCP_SA_ENV_VAR** - the environmental variable
  name whose base 64 encoded JSON value is the
  [Google credential](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
  for the GCP
  [service account](https://cloud.google.com/docs/authentication/#service_accounts)
  to be used when querying the Google Cloud DNS (optional).

  Specifying both `gcp_service_account_json` and `gcp_service_account_file`
  will generate an error.

  If GCP service account credentials are not provided by either
  `gcp_service_account_json` or `gcp_service_account_file`, then CoreDNS tries
  to obtain GCP credentials using the strategy "Application Default Credentials"
  detailed below.

* `gcp_service_account_file` **GCP_CREDENTIALS_FILE**  - the Google
  [service account](https://cloud.google.com/docs/authentication/#service_accounts)
  credentials file that contains the
  [Google credential](https://cloud.google.com/iam/docs/creating-managing-service-account-keys)
   to be used when querying the Google Cloud DNS (optional).

  Specifying both `gcp_service_account_json` and `gcp_service_account_file`
  will generate an error.

  If GCP service account credentials are not provided by either
  `gcp_service_account_json` or `gcp_service_account_file`, then CoreDNS tries
  to obtain GCP credentials using the strategy "Application Default Credentials"
  detailed below.

* `fallthrough` If zone matches and no record can be generated, pass request to
  the next plugin.  If **[ZONES...]** is omitted, then fallthrough happens for
  all zones for which the plugin is authoritative. If specific zones are listed
  (for example `in-addr.arpa` and `ip6.arpa`), then only queries for
  those zones will be subject to fallthrough.

* **ZONES** zones it should be authoritative for. If empty, the zones from the
  configuration block.

## Examples

Enable gcpdns with GCP Application Default Credentials:

~~~ txt
. {
    gcpdns example.org.:my-project/my-zone-name
    forward . 10.0.0.1
}
~~~

Enable gcpdns with explicit GCP service account credentials, base 64 encoded,
stored in the environmental variable `DNS_SERVICE_ACCOUNT`:

~~~ txt
. {
    gcpdns example.org.:my-project/my-zone-name {
      gcp_service_account_json DNS_SERVICE_ACCOUNT
    }
}
~~~

Enable gcpdns with explicit GCP service account credentials stored in the file
`/etc/coredns/gcp-service-account.json`:

~~~ txt
. {
    gcpdns example.org.:my-project/my-zone-name {
      gcp_service_account_file /etc/coredns/gcp-service-account.json
    }
}
~~~

Enable gcpdns with fallthrough:

~~~ txt
. {
    gcpdns example.org.:my-project/my-org-zone-name example.gov.:my-project/my-gov-zone-name {
      fallthrough example.gov.
    }
}
~~~

Enable gcpdns with multiple hosted zones with the same domain:

~~~ txt
. {
    gcpdns example.org.:my-private-project/my-private-zone-name example.org.:my-public-project/my-public-zone-name
}
~~~

## Application Default Credentials

GCP Cloud DNS client uses a strategy called "Application Default Credentials"
to find GCP credentials.

It looks for credentials in the following places, preferring the first location
found:

1. A JSON file whose path is specified by the
   `GOOGLE_APPLICATION_CREDENTIALS` environment variable.
2. A JSON file in a location known to the `gcloud` command-line tool.
   On Windows, this is `%APPDATA%/gcloud/application_default_credentials.json`.
   On other systems, `$HOME/.config/gcloud/application_default_credentials.json`.
3. On Google App Engine standard first generation runtimes (<= Go 1.9) it uses
   the `appengine.AccessToken` function.
4. On Google Compute Engine, Google App Engine standard second generation
   runtimes (>= Go 1.11), and Google App Engine flexible environment, it
   fetches credentials from the metadata server.
