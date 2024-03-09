#!/bin/bash

# Test UTF8 support, including usernames and domains.
# Also test normalization: the destinations will have non-matching
# capitalizations.

set -e
. "$(dirname "$0")/../util/lib.sh"

init

generate_certs_for ñoños

# Intentionally have a config directory for upper case; this should be
# normalized to lowercase internally (and match the cert accordingly).
add_user ñandú@ñoñOS araño
add_user ñangapirí@ñoñOS antaño

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1465

# Use a mix of upper and lower case in the from, to, and username, to check
# normalization is well handled end-to-end.
smtpc --addr=localhost:1465 \
	--server_cert=config/certs/ñoños/fullchain.pem \
	--user=ñanDÚ@ñoños --password=araño \
	Ñangapirí@Ñoños < content

# The MDA should see the normalized users and domains, in lower case.
wait_for_file .mail/ñangapirí@ñoños
mail_diff content .mail/ñangapirí@ñoños

success
