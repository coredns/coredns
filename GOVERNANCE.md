# CoreDNS Governance

## Principles

The CoreDNS community adheres to the following principles:

- Open: CoreDNS is open source, advertised on [our website](https://coredns.io/community)
- Welcoming and respectful: See [Code of Conduct](#code-of-conduct), below.
- Transparent and accessible: Work and collaboration are done in public.
- Merit: Ideas and contributions are accepted according to their technical merit and alignment with project objectives, scope, and design principles.


## Expectations from Maintainers

Everyone carries water...

Making a community work requires input/effort from everyone.
Maintainers should actively participate in Pull Request reviews.
Maintainers are expected to respond to assigned Pull Requests in a *reasonable* time frame,
either providing insights, or assign the Pull Requests to other maintainers.

Every Maintainer is listed in the top-level [OWNERS](OWNERS)
file, with their Github handle and a possibly obfuscated email address.
Everyone in the `approvers` list is a Maintainer.

A Maintainer is also listed in a plugin specific OWNER file.

A Maintainer should be a member of `maintainer@coredns.io`, although this is not a hard requirement.

## Becoming a Maintainer

On successful merge of a significant pull request, any current maintainer can reach
to the author and ask them if they are willing to become a CoreDNS maintainer.

## Changes in Maintainership

If a Maintainer feels she/he can not fulfill the "Expectations from Maintainers", they are free to
step down.

The CoreDNS organization will never forcefully remove a current Maintainer, unless a maintainer
fails to adhere to the [Code of Conduct](https://github.com/coredns/coredns/blob/master/CODE-OF-CONDUCT.md),
or meet the principles of the CoreDNS community as defined in this document.


## Other Projects

The CoreDNS organization is open to receive new sub-projects under its umbrella. To accept a project
into the __CoreDNS__ organization, it has to meet the following criteria:

- Must be licensed under the terms of the Apache License v2.0.
- Must be related to one or more scopes of the CoreDNS ecosystem:
  - CoreDNS project artifacts (website, deployments, CI, etc ...)
  - External plugins
  - Other DNS related processing
- Must be supported by a Maintainer.

The submission process starts as a Pull Request or Issue on the
[coredns/coredns](https://github.com/coredns/coredns) repository with the required information
mentioned above. Once a project is accepted, it's considered a __CNCF sub-project under the umbrella
of CoreDNS__

## Decision making process

Decisions are build on consensus between maintainers.
Proposals and ideas can either be submitted for agreement via an github issue or by sending an email to `maintainer@coredns.io`

In general, we prefer that technical issues and maintainer membership are amicably worked out between the persons involved.
If a dispute cannot be decided independently, the maintainers can be called in to decide an issue.
If the maintainers themselves cannot decide an issue, the issue will be resolved by voting.

For formal votes, the following should be added to the relevant github issue or PR:
* A specific statement of what is being voted on.
* The voting period - a suitable amount of time during which voting will occur.

A link to that issue or PR should be sent to `maintainer@coredns.io`.

Maintainers should indicate their yes/no vote (or respectively +1/-1) on that issue or PR.

After the voting period is over, the votes will be tallied and the outcome noted.

A 2/3 majority of cast votes is needed for the statement to be approved.

Each maintainer weighs one vote.<br>
Miek Gieben (@miekg), as the founder of CoreDNS, weighs two votes.

## Code of Conduct

the [CoreDNS Code of Conduct](https://github.com/coredns/coredns/CODE-OF-CONDUCT.md) is aligned with the CNCF Code of Conduct.

## Credits

Sections of this documents have been borrowed from [Fluentd](https://github.com/fluent/fluentd/blob/master/GOVERNANCE.md) and [Envoy](https://github.com/envoyproxy/envoy/blob/master/GOVERNANCE.md) projects.
