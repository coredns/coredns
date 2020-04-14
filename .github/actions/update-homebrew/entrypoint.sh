#!/usr/bin/env bash
set -euo pipefail

# Exit when no release tag (GITHUB_REF) is provided
[[ -z "$GITHUB_REF" ]] && exit

version=${GITHUB_REF:1}
sha256="$(curl -sLf "https://github.com/coredns/coredns/releases/download/v${version}/coredns_${version}_darwin_amd64.tgz.sha256")"

# Exit when no hash could be retrieved (e.g. release without published assets)
[[ -z "$sha256" ]] && exit

# The numbers at the beginning indicate the lines in `coredns.rb`,
# just to make sure to not replace some other occurrence by accident.
sed -i \
  -e "4s|url.*|url \"https://github.com/coredns/coredns/releases/download/v${version}/coredns_${version}_darwin_amd64.tgz\"|" \
  -e "5s|version.*|version \"$version\"|" \
  -e "6s|sha256.*|sha256 \"${sha256%% *}\"|" \
  "${CHECKOUT_PATH:-.}/HomebrewFormula/coredns.rb"
