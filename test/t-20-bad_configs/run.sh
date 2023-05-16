#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

mkdir -p .logs

if chasquid --config_dir=doesnotexist > .chasquid-doesnotexist.out 2>&1; then
	fail "chasquid should not start without a config"
fi

# Create this empty directory. We can't use a .keep file because that defeats
# the purpose of the test.
mkdir -p c-04-no_cert_dirs/certs/

# Generate certs for the tests that need them.
for i in c-05-no_addrs c-06-bad_maillog c-07-bad_domain_info \
	c-08-bad_sts_cache c-09-bad_queue_dir ;
do
	CONFDIR=$i/ generate_certs_for testserver
done

for i in c-*; do
	if chasquid --config_dir="$i" > ".chasquid-$i.out" 2>&1; then
		echo "$i failed; output:"
		echo
		cat ".chasquid-$i.out"
		echo
		fail "$i: chasquid should not start with this invalid config"
	fi

	# Test that they failed as expected, and not by chance/unrelated error.
	if ! tail -n 1 ".chasquid-$i.out" \
	   | grep -q -E "$(cat "$i/.expected-error")"; then
		echo "$i failed"
		echo "expected last line to match:"
		echo "    '$(cat "$i/.expected-error")'"
		echo "got last line:"
		echo "    '$(tail -n 1 ".chasquid-$i.out")'"
		echo
		fail "$i: chasquid did not fail as expected"
	fi
done

success
