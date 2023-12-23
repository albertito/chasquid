#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init

generate_certs_for testserver
add_user user@testserver secretpassword

mkdir -p .logs .mbox
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

FAILED=0
for i in *.cmy; do
	if ! chamuyero "$i" > ".logs/$i.log" 2>&1 ; then
		echo "test $i failed, see .logs/$i.log"
		echo
		echo "last lines of the log:"
		tail -n 10 ".logs/$i.log" | sed 's/^/  /g'
		echo
		FAILED=1
		continue
	fi

	# Some tests do email delivery, this allows us to verify the results.
	if [ -f "$i.verify" ]; then
		wait_for_file .mail/user@testserver
		cp .mail/user@testserver ".mbox/$i.mbox"
		if ! mail_diff "$i.verify" .mail/user@testserver \
			> ".mbox/$i.diff" ;
		then
			echo "test $i failed, because it had a mail diff"
			echo
			echo "mail diff:"
			sed 's/^/  /g' ".mbox/$i.diff"
			echo
			FAILED=1
		fi
	fi
done

if [ $FAILED == 1 ]; then
	fail "got at least one error"
fi
success
