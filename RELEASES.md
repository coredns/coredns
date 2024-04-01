# Releases Guide

Here, we will briefly understand the release plan and versioning steps. 
This will cover the release process, major and minor releases, backporting the PR and manual release steps.

If there is something that you require or this document leaves out, please
reach out by [filing an issue](https://github.com/coredns/coredns/issues).

## Releases

All the major and minor releases will be made from the master. The tag will be of the format `v<major>.<minor>.<patch>`.

After a minor release, a branch will be created with the format of `release-<major>.<minor>` from the minor tag. Once the branch is ready, all the patches will be released from that branch. 
For example, once we release `v1.10.0`, a branch will be created as `release-1.10` with the following `v1.10.0` tag. Next, all future patches will be done against that branch. i.e. `v1.10.1, v1.10.2, ...`.

## Next Release

The next _minor_ release will be tracked as a GitHub Milestone.

## Coredns Support

The support will be identified with several states:

- __*Active*__: The release is a stable branch that is currently supported and accepting patches.
- __*End of Life*__: The release branch is no longer supported, and no new patches will be accepted.

Currently, we are planning to provide maintenance support for the two latest release branches **(N, N-1)**. Examples: If the latest release is 1.10, we will support 1.9 and 1.10.  If the latest release is 2.0, then we will support 2.0, and the most recent release branch of 1.X. 

Maintenance releases will be supported for upto 1 year until the end of life is announced for the branch i.e. v1.9 **(N-2)** will be EOL when v1.11 (N) is released.
These branches will accept bug reports and backports until the end of life.

## Backporting

If there are important bug or security fixes that need to be backported please let us know in one of three ways:
- Open an issue.
- Open a PR with a cherry-picked change from the master branch.
- Open a PR with a ported fix.

If there is no existing fix in the master, you should first fix it in the master. Once the PR is merged, back port the fix to the supported release branches.
If the issue is only in the release branch and not in the master, then open a PR and make the specific changes on the master branch to fix the issue.
