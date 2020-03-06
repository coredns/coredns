#!/usr/bin/env bash
#
# Description: Fix up the file mtimes based on the git log.

set -u -o pipefail

if [[ ! -f 'coredns.1.md' ]]; then
  echo 'ERROR: Must be run from the top of the git repo.'
  exit 1
fi

for file in coredns.1.md corefile.5.md plugin/*/README.md; do
  time=$(git log --pretty=format:%cd -n 1 --date='format:%Y%m%d%H%M.%S' "${file}")
  touch -m -t "${time}" "${file}"
done
