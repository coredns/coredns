#!/usr/bin/env bash

function help {
    echo "Build a new version of the docker builder image"
    usage
}

function usage {
    echo "Usage: "
    echo "  --dry-run: if set, the images will not be pushed"
    echo "  --no-sign: if set, the images will not be signed (useful if ddsign is not installed on the machine)"
    echo "  -h, --help: show this help and exit"
}

function error {
    echo "$1"
    echo ""
    usage
    exit 2
}

set -euo pipefail

REGISTRY="registry.ddbuild.io/coredns/builder"
TAG=$(git rev-parse --short --verify HEAD)
ARCHS=(amd64 arm64)
PLATFORMS=(linux/amd64 linux/arm64)

PUSH=false
SKIP_SIGN=false

while [[ $# -gt 0 ]]; do
  key="$1"

  case $key in
    --push)
      PUSH=true
      shift
      ;;
    --no-sign)
      SKIP_SIGN=true
      shift
      ;;
    -h|--help)
      help
      exit 0
      ;;
    *)    # unknown option
      error "Unknown option $key"
      ;;
  esac
done

# Only sign if the image is being pushed
if [ "$PUSH" != true ]; then
  SKIP_SIGN=true

fi

# Validate tag. Cannot be empty as this would be interpreted as latest
if [ -z "$TAG" ]; then
    error "Could not determine the tag"
fi

METADATA_FILE=$(mktemp)
if [ "$PUSH" = true ]; then
    docker buildx build . --platform $PLATFORMS -t $REGISTRY:$TAG --metadata-file ${METADATA_FILE} --push;
else
    docker buildx build . --platform $PLATFORMS -t $REGISTRY:$TAG --metadata-file ${METADATA_FILE};
fi

if [ "$SKIP_SIGN" != true ]; then
  ddsign sign $REGISTRY:$TAG --docker-metadata-file ${METADATA_FILE}
fi

if [ "$PUSH" = true ]; then
    echo "Pushed image $REGISTRY:$TAG successfully"
fi