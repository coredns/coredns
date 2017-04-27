#!/bin/sh

set -ex

# Wraps autotools.sh, but each binary crashes if it exhibits undefined behavior. See
# http://releases.llvm.org/3.8.0/tools/clang/docs/UndefinedBehaviorSanitizer.html

# Install a more recent clang than default:
sudo apt-get update
sudo apt-get install -y --no-install-recommends clang-3.8 llvm-3.8-dev
export CC=clang-3.8
export CXX=clang++-3.8

# Set the undefined behavior flags. This crashes on all undefined behavior except for
# undefined casting, aka "vptr".
#
# TODO: fix undefined vptr behavior and turn this option back on.
export CFLAGS="-fsanitize=undefined -fno-sanitize-recover=undefined"
# Builds without optimization and with debugging symbols for making crash reports more
# readable.
export CFLAGS="${CFLAGS} -O0 -ggdb3"
export CXXFLAGS="${CFLAGS}"
export UBSAN_OPTIONS=print_stacktrace=1

# llvm-symbolizer must be on PATH, but the above installation instals a binary called
# "llvm-symbolizer-3.8", not "llvm-symbolizer". This fixes that with a softlink in a new
# directory.
CLANG_PATH="$(mktemp -d)"
trap "rm -rf ${CLANG_PATH}" EXIT
ln -s "$(whereis llvm-symbolizer-3.8  | rev | cut -d ' ' -f 1 | rev)" \
  "${CLANG_PATH}/llvm-symbolizer"
export PATH="${CLANG_PATH}:${PATH}"
llvm-symbolizer -version

build/docker/scripts/autotools.sh $*
