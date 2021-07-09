#!/bin/bash
#
# Test integration with dkimpy.

set -e
. $(dirname ${0})/../util/lib.sh

init
check_hostaliases

# Check if dkimpy tools are installed in /usr/bin, and driusan/dkim is
# installed somewhere else in $PATH.
#
# Unfortunately we need both because dkimpy's dkimverify lacks the features
# needed to use it in integration testing.
#
# We need to run them and check the help because there are other binaries with
# the same name.
# This is really hacky but the most practical way to handle it, since they
# both have the same binary names.
if ! /usr/bin/dkimsign --help 2>&1 | grep -q -- --identity; then
	skip "/usr/bin/dkimsign is not dkimpy's"
fi
if ! dkimverify --help 2>&1 < /dev/null | grep -q -- "-txt string"; then
	skip "dkimverify is not driusan/dkim's"
fi

generate_certs_for testserver
( mkdir -p .dkimcerts; cd .dkimcerts; dknewkey private > log 2>&1 )

add_user user@testserver secretpassword
add_user someone@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

# Authenticated: user@testserver -> someone@testserver
# Should be signed.
run_msmtp someone@testserver < content
wait_for_file .mail/someone@testserver
mail_diff content .mail/someone@testserver
grep -q "DKIM-Signature:" .mail/someone@testserver

# Verify the signature manually, just in case.
# NOTE: This is using driusan/dkim instead of dkimpy, because dkimpy can't be
# overriden to get the DNS information from anywhere else (text file or custom
# DNS server).
dkimverify -txt .dkimcerts/private.dns < .mail/someone@testserver

# Save the signed mail so we can verify it later.
# Drop the first line ("From blah") so it can be used as email contents.
tail -n +2 .mail/someone@testserver > .signed_content

# Not authenticated: someone@testserver -> someone@testserver
smtpc.py --server=localhost:1025 < .signed_content

# Check that the signature fails on modified content.
echo "Added content, invalid and not signed" >> .signed_content
if smtpc.py --server=localhost:1025 < .signed_content 2> /dev/null; then
	fail "DKIM verification succeeded on modified content"
fi

success
