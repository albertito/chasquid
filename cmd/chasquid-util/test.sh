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

if ! r user-remove user@domain > /dev/null; then
	echo user-remove failed
	exit 1
fi
check_userdb

if r authenticate user@domain --password=passwd > /dev/null; then
	echo authenticate for removed user worked
	exit 1
fi

touch '.data/domaininfo/s:dom%C3%A1in'
if ! r domaininfo-remove domáin; then
	echo domaininfo-remove failed
	exit 1
fi
if [ -f '.data/domaininfo/s:dom%C3%A1in' ]; then
	echo domaininfo-remove did not remove file
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
if ! ( echo "$C" | grep -E -q "hostname:.*\"$HOSTNAME\"" ); then
	echo print-config failed
	echo output: "$C"
	exit 1
fi

if r aliases-add alias2@domain target > /dev/null; then
	A=$(grep alias2 .config/domains/domain/aliases)
	if [ "$A" != "alias2: target" ]; then
		echo aliases-add failed
		echo output: "$A"
		exit 1
	fi
fi

if r aliases-add alias2@domain target > /dev/null; then
	echo aliases-add on existing alias worked
	exit 1
fi

if r aliases-add alias3@notexist target > /dev/null; then
	echo aliases-add on non-existing domain worked
	exit 1
fi

success
