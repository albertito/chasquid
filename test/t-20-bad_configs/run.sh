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
	c-08-bad_sts_cache c-09-bad_queue_dir c-10-empty_listening_addr \
	c-11-bad_dkim_key c-12-bad_users c-13-bad_aliases;
do
	CONFDIR=$i/ generate_certs_for testserver
done

# Adjust the name of the dkim key file in c-11-bad_dkim_key.
# `go get` rejects repos that have files with ':', so as a workaround we store
# a compatible file name in the repo, and copy it before testing.
cp c-11-bad_dkim_key/domains/testserver/dkim__selector.pem \
	c-11-bad_dkim_key/domains/testserver/dkim:selector.pem

# For the bad_users and bad_aliases test, make the relevant file unreadable.
chmod -rw c-12-bad_users/domains/testserver/users
chmod -rw c-13-bad_aliases/domains/testserver/aliases

for i in c-*; do
	if chasquid --config_dir="$i" > ".chasquid-$i.out" 2>&1; then
		echo "$i failed; output:"
		echo
		cat ".chasquid-$i.out"
		echo
		fail "$i: chasquid should not start with this invalid config"
	fi

	# Test that they failed as expected, and not by chance/unrelated error.
	# Look in the last 4 lines, because the fatal error may not be in the
	# very last one due to asynchronous logging.
	if ! tail -n 4 ".chasquid-$i.out" \
	   | grep -q -E "$(cat "$i/.expected-error")"; then
		echo "$i failed"
		echo "expected last 4 lines to contain:"
		echo "    '$(cat "$i/.expected-error")'"
		echo "got last 4 lines:"
		tail -n 4 ".chasquid-$i.out" | sed -e 's/^/    /g'
		echo
		fail "$i: chasquid did not fail as expected"
	fi
done

# Give permissions back, to avoid annoying git messages.
chmod +rw c-12-bad_users/domains/testserver/users
chmod +rw c-13-bad_aliases/domains/testserver/aliases

success
