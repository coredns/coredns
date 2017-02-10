#!/bin/sh
set -x -e
set -- /coredns "$@"

if [ -z "$COREFILE" ]; then
    exec "$@"
else
    exec echo "$COREFILE" | "$@" -conf stdin
fi
