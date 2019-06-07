#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

fail_on_output() {
  tee /dev/stderr | (! read)
}

# - Check that module is tidy.
if GO111MODULE=on go help mod >& /dev/null; then
  GO111MODULE=on go mod tidy && \
    git status --porcelain 2>&1 | fail_on_output || \
    (git status; git --no-pager diff; exit 1)
fi

# - Check vendor file is synced up with go.mod
GO111MODULE=on go mod vendor
vendorfiles=$(git diff vendor)
if [ -n "$vendorfiles" ]; then
   echo "vendor files need be updated"
   echo "----------------------------"
   git diff vendor
   echo "----------------------------"
   exit 1
fi
