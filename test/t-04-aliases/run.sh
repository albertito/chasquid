#!/bin/bash

set -e
. "$(dirname "$0")/../util/lib.sh"

init
check_hostaliases

generate_certs_for testserver
add_user user@testserver secretpassword

mkdir -p .logs
chasquid -v=2 --logfile=.logs/chasquid.log --config_dir=config &
wait_until_ready 1025

function send_and_check() {
	run_msmtp "$1@testserver" < content
	shift
	for i in "$@"; do
		wait_for_file ".mail/$i@testserver"
		mail_diff content ".mail/$i@testserver"
		rm -f ".mail/$i@testserver"
	done
	if ! [ -z "$(ls .mail/)" ]; then
		fail "unexpected mail was delivered: $(ls .mail/)"
	fi
}

# Remove the hooks that could be left over from previous failed tests.
rm -f config/hooks/alias-resolve

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

# Set up the hooks.
mkdir -p config/hooks/
cp alias-resolve-hook config/hooks/alias-resolve

# Test email aliases via the hook.
send_and_check vicuña juan jose
send_and_check vi.cu.ña juan jose
send_and_check vi.cu.ña+abc juan jose
send_and_check vic.uña+abc uña

# Test the pipe alias separately.
rm -f .data/pipe_alias_worked
run_msmtp ñandú@testserver < content
wait_for_file .data/pipe_alias_worked
mail_diff content .data/pipe_alias_worked

# Test when alias-resolve exits with an error
if run_msmtp roto@testserver < content 2> .logs/msmtp.out; then
	fail "expected delivery to roto@ to fail, but succeeded"
fi

# Test a non-existent alias.
if run_msmtp nono@testserver < content 2> .logs/msmtp.out; then
	fail "expected delivery to nono@ to fail, but succeeded"
fi

# Test chasquid-util's ability to do alias resolution talking to chasquid.
# We use chamuyero for convenience, so we can match the output exactly.
for i in *.cmy; do
	if ! chamuyero "$i" > "$i.log" 2>&1 ; then
		echo "$i failed, log follows"
		cat "$i.log"
		exit 1
	fi
done

# Remove the hooks, leave a clean state.
rm -f config/hooks/alias-resolve

success
