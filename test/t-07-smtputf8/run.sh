#!/bin/bash

# Test UTF8 support, including usernames and domains.
# Also test normalization: the destinations will have non-matching
# capitalizations.

set -e
. $(dirname ${0})/../util/lib.sh

init

skip_if_python_is_too_old

generate_certs_for ñoños

# Intentionally have a config directory for upper case; this should be
# normalized to lowercase internally (and match the cert accordingly).
add_user ñoñOS ñangapirí antaño

# Python doesn't support UTF8 for auth, use an ascii user and domain.
add_user nada nada nada

mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

# The envelope from and to are taken from the content, and use a mix of upper
# and lower case.
smtpc.py --server=localhost:1025 --user=nada@nada --password=nada \
	< content

# The MDA should see the normalized users and domains, in lower case.
wait_for_file .mail/ñangapirí@ñoños
mail_diff content .mail/ñangapirí@ñoños

success
