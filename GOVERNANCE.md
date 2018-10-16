# CoreDNS Governance

## Principles

The CoreDNS community adheres to the following principles:

- Open: CoreDNS is open source. See repository guidelines, below.
- Welcoming and respectful: See Code of Conduct, below.
- Transparent and accessible: Work and collaboration are done in public.
- Merit: Ideas and contributions are accepted according to their technical merit and alignment with project objectives, scope, and design principles.


## Expectations from Maintainers

"Every one carries water."

Making a community work requires input/effort from every one. Maintainers should actively
participate in Pull Request reviews. Maintainers are expected to respond to assigned Pull Requests
in a *reasonable* time frame, either providing insights, or assign the Pull Requests to other
maintainers.

Every Maintainer is listed in the top-level [OWNERS](https://github.com/coredns/coredns/OWNERS)
file, with their Github handle and an (possible obfuscated) email address. Every one in the
`reviewers` list is a Maintainer.

A Maintainer is also listed in a plugin specific OWNER file.

A Maintainer should be a member of `maintainer@coredns.io`, although this is not a hard requirement.
A Maintainer that hasn't been active in the CoreDNS repository for 12 months is considered inactive.

## Becoming a Maintainers

On successful completion (it was merged) of a (large) pull request, any current maintainer can reach
to the person behind the pull request and ask them if they are willing to become a CoreDNS
maintainer.

## Changes in Maintainership

If a Maintainer feels she/he can not fulfill the "Expectations from Maintainers", they are free to
step down.

The CoreDNS organization will never forcefully remove a current Maintainer, unless a maintainer
fails to meet the principles of CoreDNS community.


## Other Projects

The CoreDNS organization is open to receive new sub-projects under its umbrella. To accept project
into the __CoreDNS__ organization, it has to met the following criteria:

- Licensed under the terms of the Apache License v2.0.
- Related to one or more scopes of CoreDNS ecosystem:
  - CoreDNS project artifacts (website, deployments, CI, etc ...)
  - External plugins
  - other DNS processing related
- Be supported by a Maintainer.

The submission process starts as a Pull Request or Issue on the
[coredns/coredns](https://github.com/coredns/coredns) repository with the required information
mentioned above. Once a project is accepted, it's considered a __CNCF sub-project under the umbrella
of CoreDNS__

## Decision making process

Decisions are build on consensus between maintainers.
Proposal and ideas can either be submitted for agreement via an github issue or by sending an email to `maintainer@coredns.io`

In general, we prefer that technical issues and maintainer membership are amicably worked out between the persons involved.
If a dispute cannot be decided independently, the maintainers can be called in to decide an issue.
If the maintainers themselves cannot decide an issue, the issue will be resolved by voting.

For formal votes, a specific statement of what is being voted on, and in which delay (a suitable amount of time),
should be added to the relevant github issue or PR, and a link to that issue
or PR sent to `maintainer@coredns.io`.

Maintainers should indicate their yes/no vote (or respectively +1/-1) on that issue or PR,
and after the delay is expired, the votes will be tallied and the outcome noted.

A 2/3 majority vote is needed for the statement to be approved.

Each maintainer weighs one vote.<br>
Miek Gieben (@miekg), as the historical owner of CoreDNS, weighs two votes.

## Code of Conduct

CoreDNS follows the [CNCF Code of Conduct](https://github.com/coredns/coredns/CODE-OF-CONDUCT.md).

