#!/bin/bash

set -e
. "$(dirname "$0")/../../test/util/lib.sh"

init

# Build the binary once, so we can use it and launch it in chamuyero scripts.
# Otherwise, we not only spend time rebuilding it over and over, but also "go
# run" masks the exit code, which is something we care about.
if [ "${GOCOVERDIR}" != "" ]; then
	GOFLAGS="-cover -covermode=count -o dovecot-auth-cli $GOFLAGS"
fi

# shellcheck disable=SC2086
go build $GOFLAGS -tags="$GOTAGS" .

if ! ./dovecot-auth-cli lalala 2>&1 | grep -q "invalid arguments"; then
	echo "cli worked with invalid arguments"
	exit 1
fi

for i in *.cmy; do
	if ! chamuyero "$i" > "$i.log" 2>&1 ; then
		echo "# Test $i failed, log follows"
		cat "$i.log"
		exit 1
	fi
done

success
exit 0
