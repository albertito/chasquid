#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

skip_if_python_is_too_old

generate_certs_for ñoños
add_user ñoños ñangapirí antaño

# Python doesn't support UTF8 for auth, use an ascii user and domain.
add_user nada nada nada

mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

smtpc.py --server=localhost:1025 --user=nada@nada --password=nada \
	< content

wait_for_file .mail/ñangapirí@ñoños
mail_diff content .mail/ñangapirí@ñoños

success
