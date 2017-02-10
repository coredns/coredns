#!/bin/sh
set -x -e
set -- /coredns "$@"

if [ -z "$CORE_FILE" ]; then
    exec "$@"
else
    exec echo "$CORE_FILE" | "$@" -conf stdin
fi
