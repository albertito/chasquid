#!/bin/bash

set -e
. "$(dirname "$0")/util/lib.sh"

init

FAILED=0

for i in stress-*; do
	echo "$i ..."
	setsid -w "$i/run.sh"
	FAILED=$(( FAILED + $? ))
	echo
done

exit $FAILED
