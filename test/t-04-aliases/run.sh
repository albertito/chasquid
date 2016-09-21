#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init

generate_certs_for testserver
add_user testserver user secretpassword

mkdir -p .logs
chasquid -v=2 --log_dir=.logs --config_dir=config &
wait_until_ready 1025

function send_and_check() {
		run_msmtp $1@testserver < content
		shift
		for i in $@; do
				wait_for_file .mail/$i@testserver
				mail_diff content .mail/$i@testserver
				rm -f .mail/$i@testserver
		done
}

# Test email aliases.
send_and_check pepe jose
send_and_check joan juan
send_and_check pitanga ñangapirí
send_and_check añil azul índigo

# Test suffix separators and drop characters.
send_and_check a.ñi_l azul índigo
send_and_check añil-blah azul índigo
send_and_check añil+blah azul índigo

# Test the pipe alias separately.
rm -f .data/pipe_alias_worked
run_msmtp tubo@testserver < content
wait_for_file .data/pipe_alias_worked
mail_diff content .data/pipe_alias_worked


success
