#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init

generate_certs_for testserver
add_user user@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

FAILED=0
for i in *.cmy; do
	if ! chamuyero "$i" > ".logs/$i.log" 2>&1 ; then
		echo "test $i failed, see .logs/$i.log"
		FAILED=1
	fi
done

if [ $FAILED == 1 ]; then
	fail "got at least one error"
fi
success
