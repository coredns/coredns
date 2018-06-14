# Security Release Process

CoreDNS project has adopted this security disclosures and response policy to ensure a responsible handle of critical issues.

**Table of Contents**

__TOC__


## CoreDNS Security Team

Security vulnerabilities should be handled quickly and sometimes privately. 
The primary goal of this process is to reduce the total time users are vulnerable to publicly known exploits.

The CoreDNS Security Team is responsible for organizing the entire response including internal communication and external disclosure. 

The initial CoreDNS Security Team will consist of volunteers, usual contributors to CoreDNS project.
These are the people who have been involved in the initial discussion and volunteered:

- Miek Gieben (**[@miekg](https://github.com/miekg)**) `<miek@google.com>`
- Francois Tur (**[@fturib](https://github.com/fturib)**) `<ftur@infoblox.com>`
- ????

## Disclosures

### Private Disclosure Processes

If you find a security vulnerability or any security related issues, 
please DO NOT file a public issue - that means do not create a Github issue, 
instead send your report privately to security@coredns.io. 
Security reports are greatly appreciated and we will publicly thank you for it.

### Public Disclosure Processes

If you know of a publicly disclosed security vulnerability please IMMEDIATELY email security@coredns.io 
to inform the CoreDNS Security Team about the vulnerability so we start the patch, release, and communication process.

If possible we will ask the person making the public report if the issue can be handled via a private disclosure process. 
If the reporter denies, we will move swiftly with the fix and release process. 
In extreme cases you can ask GitHub to delete the issue but this generally isn't necessary and is unlikely to make a public disclosure less damaging.

## Patch, Release, and Public Communication

For each vulnerability, CoreDNS Security Team will evaluate the impact on CoreDNS project.
If CoreDNS Security Team estimates that a release or patch release of CoreDNS is needed to fix this vulnerability.
Once it is established that a Fix i needed, the Team coordinate and organize that Security Patch Release.

All of the timelines below are suggestions and assume a Private Disclosure.
If the Team is dealing with a Public Disclosure all timelines become ASAP. 
If the fix relies on another upstream project's disclosure timeline, that will adjust the process as well.
We will work with the upstream project to fit their timeline and best protect our users.


### Fix Development Process

These steps should be completed within the 1-7 days of Disclosure.
CoreDNS Security Team will work to develop a fix or mitigation.   

### Fix Disclosure Process

With the Fix Development underway the CoreDNS Security Team needs to come up with an overall communication plan for the wider community. 
This Disclosure process should begin after the Team has developed a fix or mitigation 
so that a realistic timeline can be communicated to users.

**Disclosure of Forthcoming Fix to Users** (Completed within 1-7 days of Disclosure)

- CoreDNS Security Team will create a github issue in CoreDNS project to inform users that a security vulnerability 
has been disclosed and that a fix will be made available, with an estimation of the Release Date. 
It will include any mitigating steps users can take until a fix is available.

**Optional Fix Disclosure to Private Integrators List** (Completed within 1-14 days of Disclosure):

- CoreDNS Security Team will make a determination if an issue is critical enough to require early disclosure to integrators. 
Generally this Private Integrator Disclosure process should be reserved for remotely exploitable or privilege escalation issues. 
Otherwise, this process can be skipped.
- The CoreDNS Security Team will email the patches to coredns-integrators-announce@coredns.io so integrators can prepare their own release to be available to users on the day of the issue's announcement. 
Integrators should read about the [Private Integrator List](#private-integrator-list) to find out the requirements for being added to this list.
- **What if an integrator breaks embargo?** The CoreDNS Security Team will assess the damage and may make the call to release earlier or continue with the plan. 
When in doubt push forward and go public ASAP.

**Fix Release Day** (Completed within 1-21 days of Disclosure)

- CoreDNS Security Team will cherry-pick all needed commits from the Master branch in order to create a new release on top of the current last version released.
- Release process will be as usual.
- CoreDNS Security Team will inform users of the release by usual means, adding information on what security issue is fixed/workarounded


## Private Integrator List

This list is intended to be used primarily to provide actionable information to
multiple integrator projects at once. This list is not intended for
individuals to find out about security issues.

### Embargo Policy

The information members receive on coredns-integrators-announce@coredns.io must not be
made public, shared, nor even hinted at anywhere beyond the need-to-know within
your specific team except with the list's explicit approval. 
This holds true until the public disclosure date/time that was agreed upon by the list.
Members of the list and others may not use the information for anything other
than getting the issue fixed for your respective distribution's users.

Before any information from the list is shared with respective members of your
team required to fix said issue, they must agree to the same terms and only
find out information on a need-to-know basis.

In the unfortunate event you share the information beyond what is allowed by
this policy, you _must_ urgently inform the security@coredns.io
mailing list of exactly what information leaked and to whom. 

If you continue to leak information and break the policy outlined here, you
will be removed from the list.

### Membership Criteria

To be eligible for the coredns-integrator-announce mailing list, your
distribution should:

1. Be an actively integrator of CoreDNS component.
2. Have a user base not limited to your own organization.
3. Have a publicly verifiable track record up to present day of fixing security
   issues.
4. Not be a downstream or rebuild of another integrator.
5. Be a participant and active contributor in the community.
6. Accept the [Embargo Policy](#embargo-policy) that is outlined above.
7. Have someone already on the list vouch for the person requesting membership
   on behalf of your distribution.

### Requesting to Join

New membership requests are sent to security@coredns.io.

In the body of your request please specify how you qualify and fulfill each
criterion listed in [Membership Criteria](#membership-criteria).

