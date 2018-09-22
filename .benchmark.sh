#!/usr/bin/env bash

set -e +o pipefail

if [ "$TRAVIS_PULL_REQUEST" != "false" ] ; then
    # Test Token Length
    echo ${#GITHUB_TOKEN}
    jq -n --arg body "$(cat .benchmark.log)" '{body: $body}' > .benchmark.json
    curl -H "Authorization: token ${GITHUB_TOKEN}" -X POST \
        --data-binary "@.benchmark.json" \
        "https://api.github.com/repos/${TRAVIS_REPO_SLUG}/issues/${TRAVIS_PULL_REQUEST}/comments"
fi
