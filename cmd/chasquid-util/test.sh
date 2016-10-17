#!/bin/bash

set -e
. $(dirname ${0})/../../test/util/lib.sh

init

go build || exit 1

function r() {
	./chasquid-util -C .config "$@"
}

function check_userdb() {
	if ! r check-userdb domain > /dev/null; then
		echo check-userdb failed
		exit 1
	fi
}


mkdir -p .config/domains/domain
touch .config/chasquid.conf

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

if ! r user-remove user@domain > /dev/null; then
	echo user-remove failed
	exit 1
fi
check_userdb

if r authenticate user@domain --password=passwd > /dev/null; then
	echo authenticate for removed user worked
	exit 1
fi

echo "alias: user@somewhere" > .config/domains/domain/aliases
A=$(r aliases-resolve alias@domain | grep somewhere)
if [ "$A" != "(email)  user@somewhere" ]; then
	echo aliases-resolve failed
	echo output: "$A"
	exit 1
fi

C=$(r print-config | grep hostname)
if [ "$C" != "hostname: \"$HOSTNAME\"" ]; then
	echo print-config failed
	echo output: "$C"
	exit 1
fi

success
