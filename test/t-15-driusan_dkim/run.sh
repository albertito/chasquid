#!/bin/bash
#
# Test integration with driusan's DKIM tools.
# https://github.com/driusan/dkim

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

for binary in dkimsign dkimverify dkimkeygen; do
	if ! command -v $binary > /dev/null; then
		skip "$binary binary not found"
	fi
done

generate_certs_for testserver
( mkdir -p .dkimcerts; cd .dkimcerts; dkimkeygen )

add_user user@testserver secretpassword
add_user someone@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1465

# Authenticated: user@testserver -> someone@testserver
# Should be signed.
smtpc --addr=localhost:1465 \
	--server_cert=config/certs/testserver/fullchain.pem \
	--user=user@testserver --password=secretpassword \
	someone@testserver < content
wait_for_file .mail/someone@testserver
mail_diff content .mail/someone@testserver
grep -q "DKIM-Signature:" .mail/someone@testserver

# Verify the signature manually, just in case.
dkimverify -txt .dkimcerts/dns.txt < .mail/someone@testserver

# Save the signed mail so we can verify it later.
# Drop the first line ("From blah") so it can be used as email contents.
tail -n +2 .mail/someone@testserver > .signed_content

# Not authenticated: someone@testserver -> someone@testserver
smtpc --addr=localhost:1025 \
	--from=someone@testserver someone@testserver < .signed_content

# Check that the signature fails on modified content.
echo "Added content, invalid and not signed" >> .signed_content
if smtpc --addr=localhost:1025 \
	--from=someone@testserver someone@testserver < .signed_content \
	> /dev/null 2>&1 ; then
	fail "DKIM verification succeeded on modified content"
fi

success
