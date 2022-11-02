#!/usr/bin/bash

jq_overwrite() {
	FILE=$1
	shift
	jq -c "$@" $FILE > $FILE.temp
	mv $FILE.temp $FILE
}

change_arch() {
	FILE=$1
	ARCH=$2
	jq_overwrite $FILE '.architecture = $arch' --arg arch $ARCH
}

ARCH=$1

tempdir=$(mktemp -d)
pushd $tempdir > /dev/null
trap "popd > /dev/null; rm -rf $tempdir" EXIT

# extract archive from stdin
tar x

# change architecture of *.json
CONFIG=$(jq -r '.[0].Config' manifest.json)
change_arch $CONFIG $ARCH

# rename *.json
set -- $(sha256sum $CONFIG)
NEWCONFIG=$1.json
mv $CONFIG $NEWCONFIG

# apply new filename to manifest.json
jq_overwrite manifest.json '.[0].Config = $config' --arg config $NEWCONFIG

# change architecture of */json
for json in */json; do
	change_arch $json $ARCH
done

# write archive to stdout
tar c *
