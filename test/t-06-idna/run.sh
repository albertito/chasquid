#!/bin/bash

set -e
. $(dirname ${0})/../util/lib.sh

init
check_hostaliases

rm -rf .data-A .data-B .mail

skip_if_python_is_too_old

# Two servers:
# A - listens on :1025, hosts srv-ñ
# B - listens on :2015, hosts srv-ü

CONFDIR=A generate_certs_for srv-ñ
CONFDIR=A add_user ñangapirí@srv-ñ antaño
CONFDIR=A add_user nadaA@nadaA nadaA

CONFDIR=B generate_certs_for srv-ü
CONFDIR=B add_user pingüino@srv-ü velóz
CONFDIR=B add_user nadaB@nadaB nadaB

mkdir -p .logs-A .logs-B

chasquid -v=2 --logfile=.logs-A/chasquid.log --config_dir=A \
	--testing__outgoing_smtp_port=2025 &
chasquid -v=2 --logfile=.logs-B/chasquid.log --config_dir=B \
	--testing__outgoing_smtp_port=1025 &

wait_until_ready 1025
wait_until_ready 2025

# Send from A to B.
smtpc.py --server=localhost:1025 --user=nadaA@nadaA --password=nadaA \
	< from_A_to_B

wait_for_file .mail/pingüino@srv-ü
mail_diff from_A_to_B .mail/pingüino@srv-ü

# Send from B to A.
smtpc.py --server=localhost:2025 --user=nadaB@nadaB --password=nadaB \
	< from_B_to_A

wait_for_file .mail/ñangapirí@srv-ñ
mail_diff from_B_to_A .mail/ñangapirí@srv-ñ

success
