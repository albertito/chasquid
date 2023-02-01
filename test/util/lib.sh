#!/bin/bash
# Library to write the shell scripts in the tests.

function init() {
	if [ "$V" == "1" ]; then
		set -v
	fi

	UTILDIR=$(realpath "$(dirname "${BASH_SOURCE[0]}")" )
	export UTILDIR

	TBASE=$(realpath "$(dirname "$0")" )
	cd "${TBASE}" || exit 1

	if [ "${RACE}" == "1" ]; then
		GOFLAGS="$GOFLAGS -race"
	fi

	# Remove the directory where test-mda will deliver mail, so previous
	# runs don't interfere with this one.
	rm -rf .mail

	# Set traps to kill our subprocesses when we exit (for any reason).
	trap ":" TERM      # Avoid the EXIT handler from killing bash.
	trap "exit 2" INT  # Ctrl-C, make sure we fail in that case.
	trap "kill 0" EXIT # Kill children on exit.
}

function chasquid() {
	if [ "${GOCOVERDIR}" != "" ]; then
		GOFLAGS="-cover -covermode=count -o chasquid $GOFLAGS"
	fi

	# shellcheck disable=SC2086
	( cd "${TBASE}/../../" || exit 1; go build $GOFLAGS -tags="$GOTAGS" . )

	# HOSTALIASES: so we "fake" hostnames.
	# PATH: so chasquid can call test-mda without path issues.
	# MDA_DIR: so our test-mda knows where to deliver emails.
	HOSTALIASES=${TBASE}/hosts \
	PATH=${UTILDIR}:${PATH} \
	MDA_DIR=${TBASE}/.mail \
		"${TBASE}/../../chasquid" "$@"
}

# Add a user with chasquid-util. Because this is somewhat cryptographically
# intensive, it can slow down the tests significantly, so most of the time we
# use the simpler add_user (below) for testing purposes.
function chasquid-util-user-add() {
	CONFDIR="${CONFDIR:-config}"
	DOMAIN=$(echo "$1" | cut -d @ -f 2)
	mkdir -p "${CONFDIR}/domains/$DOMAIN/"
	go run "${TBASE}/../../cmd/chasquid-util/chasquid-util.go" \
		-C="${CONFDIR}" \
		user-add "$1" \
		--password="$2" \
		>> .add_user_logs
}

function add_user() {
	CONFDIR="${CONFDIR:-config}"
	USERNAME=$(echo "$1" | cut -d @ -f 1)
	DOMAIN=$(echo "$1" | cut -d @ -f 2)
	USERDB="${CONFDIR}/domains/$DOMAIN/users"
	mkdir -p "${CONFDIR}/domains/$DOMAIN/"
	if ! [ -f "${USERDB}" ] || ! grep -E -q "key:.*${USERNAME}" "${USERDB}"; then
		echo "users:{ key: '${USERNAME}' value:{ plain:{ password: '$2' }}}" \
			>> "${USERDB}"
	fi
}

function dovecot-auth-cli() {
	go run "${TBASE}/../../cmd/dovecot-auth-cli/dovecot-auth-cli.go" "$@"
}

function run_msmtp() {
	# msmtp will check that the rc file is only user readable.
	chmod 600 msmtprc

	# msmtp binary is often g+s, which causes $HOSTALIASES to not be
	# honoured, which breaks the tests. Copy the binary to remove the
	# setgid bit as a workaround.
	cp -u "$(command -v msmtp)" "${UTILDIR}/.msmtp-bin"

	HOSTALIASES=${TBASE}/hosts \
		"${UTILDIR}/.msmtp-bin" -C msmtprc "$@"
}

function smtpc.py() {
	"${UTILDIR}/smtpc.py" "$@"
}

function mail_diff() {
	"${UTILDIR}/mail_diff" "$@"
}

function chamuyero() {
	"${UTILDIR}/chamuyero" "$@"
}

function generate_cert() {
	( cd "${UTILDIR}/generate_cert/" || exit 1; go build )
	"${UTILDIR}/generate_cert/generate_cert" "$@"
}

function loadgen() {
	( cd "${UTILDIR}/loadgen/" || exit 1; go build )
	"${UTILDIR}/loadgen/loadgen" "$@"
}

function conngen() {
	( cd "${UTILDIR}/conngen/" || exit 1; go build )
	"${UTILDIR}/conngen/conngen" "$@"
}

function minidns_bg() {
	( cd "${UTILDIR}/minidns" || exit 1; go build )
	"${UTILDIR}/minidns/minidns" "$@" &
	export MINIDNS=$!
}

function fexp() {
	( cd "${UTILDIR}/fexp/" || exit 1; go build )
	"${UTILDIR}/fexp/fexp" "$@"
}

function timeout() {
	MYPID=$$
	(
		sleep "$1"
		echo "timed out after $1, killing test"
		kill -9 $MYPID
	) &
}

function success() {
	echo success
}

function skip() {
	echo "skipped: $*"
	exit 0
}

function fail() {
	echo "FAILED: $*"
	exit 1
}

function check_hostaliases() {
	if ! "${UTILDIR}/check-hostaliases"; then
		skip "\$HOSTALIASES not working (probably systemd-resolved)"
	fi
}

# Wait until there's something listening on the given port.
function wait_until_ready() {
	PORT=$1

	while ! bash -c "true < /dev/tcp/localhost/$PORT" 2>/dev/null ; do
		sleep 0.01
	done
}

# Wait for the given file to exist.
function wait_for_file() {
	while ! [ -e "$1" ]; do
		sleep 0.01
	done
}

function wait_until() {
	while true; do
		if eval "$*"; then
			return 0
		fi
		sleep 0.01
	done
}

# Generate certs for the given hostname.
function generate_certs_for() {
	CONFDIR="${CONFDIR:-config}"

	# Generating certs is takes time and slows the tests down, so we keep
	# a little cache that is common to all tests.
	CACHEDIR="${TBASE}/../.generate_certs_cache"
	mkdir -p "${CACHEDIR}"
	touch -d "10 minutes ago" "${CACHEDIR}/.reference"
	if [ "${CACHEDIR}/$1/" -ot "${CACHEDIR}/.reference" ]; then
		# Cache miss (either was not there, or was too old).
		mkdir -p "${CACHEDIR}/$1/"
		(
			cd "${CACHEDIR}/$1/" || exit 1
			generate_cert -ca -validfor=1h -host="$1"
		)
	fi
	mkdir -p "${CONFDIR}/certs/$1/"
	cp -p "${CACHEDIR}/$1"/* "${CONFDIR}/certs/$1/"
}

# Check the Python version, and skip if it's too old.
# This will check against the version required for smtpc.py.
function skip_if_python_is_too_old() {
	# We need Python >= 3.5 to be able to use SMTPUTF8.
	check='import sys; sys.exit(0 if sys.version_info >= (3, 5) else 1)'
	if ! python3 -c "${check}" > /dev/null 2>&1; then
		skip "python3 >= 3.5 not available"
	fi
}

function chasquid_ram_peak() {
	# Find the pid of the daemon, which we expect is running on the
	# background somewhere within our current session.
	SERVER_PID=$(pgrep -s 0 -x chasquid)
	grep VmHWM "/proc/$SERVER_PID/status" | cut -d ':' -f 2-
}
