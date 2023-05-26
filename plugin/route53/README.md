# route53

## Name

*route53* - enables serving zone data from AWS route53.

## Description

The route53 plugin is useful for serving zones from resource record
sets in AWS route53. This plugin supports all Amazon Route 53 records
([https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/ResourceRecordTypes.html](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/ResourceRecordTypes.html)).
The route53 plugin can be used when CoreDNS is deployed on AWS or elsewhere.

## Syntax

~~~ txt
route53 [ZONE:HOSTED_ZONE_ID...] {
    aws_access_key [AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY] # Deprecated, uses other authentication methods instead.
    aws_endpoint ENDPOINT
    credentials PROFILE [FILENAME]
    fallthrough [ZONES...]
    refresh DURATION
}
~~~

*   **ZONE** the name of the domain to be accessed. When there are multiple zones with overlapping
    domains (private vs. public hosted zone), CoreDNS does the lookup in the given order here.
    Therefore, for a non-existing resource record, SOA response will be from the rightmost zone.

*   **HOSTED\_ZONE\_ID** the ID of the hosted zone that contains the resource record sets to be
    accessed.

*   **AWS\_ACCESS\_KEY\_ID** and **AWS\_SECRET\_ACCESS\_KEY** the AWS access key ID and secret access key
    to be used when querying AWS (optional). If they are not provided, CoreDNS tries to access
    AWS credentials the same way as AWS CLI - environment variables, shared credential file (and optionally
    shared config file if `AWS_SDK_LOAD_CONFIG` env is set), and lastly EC2 Instance Roles.
    Note the usage of `aws_access_key` has been deprecated and may be removed in future versions. Instead,
    user can use other methods to pass crentials, e.g., with environmental variable `AWS_ACCESS_KEY_ID` and
    `AWS_SECRET_ACCESS_KEY`, respectively.

*   `aws_endpoint` can be used to control the endpoint to use when querying AWS (optional). **ENDPOINT** is the
    URL of the endpoint to use. If this is not provided the default AWS endpoint resolution will occur.

*   `credentials` is used for overriding the shared credentials **FILENAME** and the **PROFILE** name for a
    given zone. **PROFILE** is the AWS account profile name. Defaults to `default`. **FILENAME** is the
    AWS shared credentials filename, defaults to `~/.aws/credentials`. CoreDNS will only load shared credentials
    file and not shared config file (`~/.aws/config`) by default. Set `AWS_SDK_LOAD_CONFIG` env variable to
    a truthy value to enable also loading of `~/.aws/config` (e.g. if you want to provide assumed IAM role
    configuration). Will be ignored if static keys are set via `aws_access_key`.

*   `fallthrough` If zone matches and no record can be generated, pass request to the next plugin.
    If **ZONES** is omitted, then fallthrough happens for all zones for which the plugin is
    authoritative. If specific zones are listed (for example `in-addr.arpa` and `ip6.arpa`), then
    only queries for those zones will be subject to fallthrough.

*   `refresh` can be used to control how long between record retrievals from Route 53. It requires
    a duration string as a parameter to specify the duration between update cycles. Each update
    cycle may result in many AWS API calls depending on how many domains use this plugin and how
    many records are in each. Adjusting the update frequency may help reduce the potential of API
    rate-limiting imposed by AWS.

*   **DURATION** A duration string. Defaults to `1m`. If units are unspecified, seconds are assumed.

## Examples

Enable route53 with implicit AWS credentials and resolve CNAMEs via 10.0.0.1:

~~~ txt
example.org {
	route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7
}

. {
    forward . 10.0.0.1
}
~~~

Enable route53 with explicit AWS credentials:

~~~ txt
example.org {
    route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7 {
      aws_access_key AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY # Deprecated, uses other authentication methods instead.
    }
}
~~~

Enable route53 with an explicit AWS endpoint:

~~~ txt
example.org {
    route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7 {
      aws_endpoint https://test.us-west-2.amazonaws.com
    }
}
~~~

Enable route53 with fallthrough:

~~~ txt
. {
    route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7 example.gov.:Z654321543245 {
      fallthrough example.gov.
    }
}
~~~

Enable route53 with multiple hosted zones with the same domain:

~~~ txt
example.org {
    route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7 example.org.:Z93A52145678156
}
~~~

Enable route53 and refresh records every 3 minutes
~~~ txt
example.org {
    route53 example.org.:Z1Z2Z3Z4DZ5Z6Z7 {
      refresh 3m
    }
}
~~~

## Authentication

Route53 plugin uses [AWS Go SDK](https://docs.aws.amazon.com/sdk-for-go/v1/developer-guide/configuring-sdk.html)
for authentication, where there is a list of accepted configuration methods.
Note the usage of `aws_access_key` in Corefile has been deprecated and may be removed in future versions. Instead,
user can use other methods to pass crentials, e.g., with environmental variable `AWS_ACCESS_KEY_ID` and
`AWS_SECRET_ACCESS_KEY`, respectively.
