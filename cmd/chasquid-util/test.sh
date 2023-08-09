#!/bin/bash

set -e
. "$(dirname "$0")/../../test/util/lib.sh"

init

go build || exit 1

function r() {
	./chasquid-util -C=.config "$@"
}

function check_userdb() {
	if ! r check-userdb domain > /dev/null; then
		echo check-userdb failed
		exit 1
	fi
}


mkdir -p .config/domains/domain/ .data/domaininfo
rm -f .config/chasquid.conf
echo 'data_dir: ".data"' >> .config/chasquid.conf

if ! r print-config > /dev/null; then
	echo print-config failed
	exit 1
fi

if ! r user-add user@domain --password=passwd > /dev/null; then
	echo user-add failed
	exit 1
fi
check_userdb

if ! r authenticate user@domain --password=passwd > /dev/null; then
	echo authenticate failed
	exit 1
fi

if r authenticate user@domain --password=abcd > /dev/null; then
	echo authenticate with bad password worked
	exit 1
fi

# Interactive authentication.
# Need to wrap the execution under "script" since the interaction requires an
# actual TTY, and that's a fairly portable way to do that.
if hash script 2>/dev/null; then
	if ! (echo passwd; echo passwd ) \
		| script \
			-qfec "./chasquid-util -C=.config authenticate user@domain" \
			".script-out" \
		| grep -q "Authentication succeeded";
	then
		echo interactive authenticate failed
		exit 1
	fi
fi


if ! r user-remove user@domain > /dev/null; then
	echo user-remove failed
	exit 1
fi
check_userdb

if r authenticate user@domain --password=passwd > /dev/null; then
	echo authenticate for removed user worked
	exit 1
fi

C=$(r print-config | grep hostname)
if ! ( echo "$C" | grep -E -q "hostname:.*\"$HOSTNAME\"" ); then
	echo print-config failed
	echo output: "$C"
	exit 1
fi

# Run all the chamuyero tests.
for i in *.cmy; do
	if ! chamuyero "$i" > "$i.log" 2>&1 ; then
		echo "# Test $i failed, log follows"
		cat "$i.log"
		exit 1
	fi
done

success
