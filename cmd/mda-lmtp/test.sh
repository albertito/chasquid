#!/bin/bash

set -e
. "$(dirname "$0")/../../test/util/lib.sh"

init

# Build the binary once, so we can use it and launch it in chamuyero scripts.
# Otherwise, we not only spend time rebuilding it over and over, but also "go
# run" masks the exit code, which is something we care about.
go build

for i in *.cmy; do
	if ! chamuyero "$i" > "$i.log" 2>&1 ; then
		echo "# Test $i failed, log follows"
		cat "$i.log"
		exit 1
	fi
done

success
