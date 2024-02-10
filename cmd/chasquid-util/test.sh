#!/bin/bash

set -e
. "$(dirname "$0")/../../test/util/lib.sh"

init

if [ "${GOCOVERDIR}" != "" ]; then
	GOFLAGS="-cover -covermode=count -o chasquid-util $GOFLAGS"
fi

# shellcheck disable=SC2086
go build $GOFLAGS -tags="$GOTAGS" .

function r() {
	./chasquid-util -C=.config "$@"
}

function check_userdb() {
	if ! r check-userdb domain > /dev/null; then
		echo check-userdb failed
		exit 1
	fi
}


rm -rf .config/
mkdir -p .config/domains/domain/ .data/domaininfo
echo 'data_dir: ".data"' >> .config/chasquid.conf

if ! r print-config > /dev/null; then
	fail print-config
fi

if ! r user-add interactive@domain --password=passwd > /dev/null; then
	fail user-add
fi

# Interactive authentication.
# Need to wrap the execution under "script" since the interaction requires an
# actual TTY, and that's a fairly portable way to do that.
if hash script 2>/dev/null; then
	if ! (echo passwd; echo passwd ) \
		| script \
			-qfec "./chasquid-util -C=.config authenticate interactive@domain" \
			".script-out" \
		| grep -q "Authentication succeeded";
	then
		fail interactive authentication
	fi
fi

C=$(r print-config | grep hostname)
if ! ( echo "$C" | grep -E -q "hostname:.*\"$HOSTNAME\"" ); then
	echo print-config failed
	echo output: "$C"
	exit 1
fi

rm -rf .keys/
mkdir .keys/

# Run all the chamuyero tests.
for i in *.cmy; do
	if ! chamuyero "$i" > "$i.log" 2>&1 ; then
		echo "# Test $i failed, log follows"
		cat "$i.log"
		exit 1
	fi
done

success
