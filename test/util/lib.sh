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
	if [ "${GOCOVERDIR}" != "" ]; then
		GOFLAGS="$GOFLAGS -cover -covermode=count"
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
	go-build-cached "${TBASE}/../../"

	# HOSTALIASES: so we "fake" hostnames.
	# PATH: so chasquid can call test-mda without path issues.
	# MDA_DIR: so our test-mda knows where to deliver emails.
	HOSTALIASES=${TBASE}/hosts \
	PATH=${UTILDIR}:${PATH} \
	MDA_DIR=${TBASE}/.mail \
		"${TBASE}/../../chasquid" "$@"
}

function go-build-cached() { (
	# This runs "go build" on the given directory, but only once every
	# 10s, or if the build flags/tags change.
	# Because in tests we run some of the Go programs often, this speeds
	# up the tests.
	cd "$1" || exit 1
	touch -d "10 seconds ago" .reference
	echo "-tags=$GOTAGS : $GOFLAGS" > .flags-new
	if
		! cmp -s .flags-new .flags >/dev/null 2>&1 ||
		[ "$(basename "$PWD")" -ot ".reference" ] ;
	then
		# shellcheck disable=SC2086
		go build -tags="$GOTAGS" $GOFLAGS

		# Write to .flags instead of renaming, to prevent races where
		# was .flags-new is already renamed by the time we get here.
		# Do this _after_ the build so worst case we build twice,
		# instead of having the chance to run an old binary.
		echo "-tags=$GOTAGS : $GOFLAGS" > .flags
	fi
) }


function chasquid-util() {
	# Run chasquid-util from inside the config dir, since in our tests
	# data_dir is relative to the config.
	go-build-cached "${TBASE}/../../cmd/chasquid-util/"
	CONFDIR="${CONFDIR:-config}"
	( cd "$CONFDIR" && \
	  "${TBASE}/../../cmd/chasquid-util/chasquid-util" \
		-C=. \
		"$@" \
	)
}

# Add a user with chasquid-util. Because this is somewhat cryptographically
# intensive, it can slow down the tests significantly, so most of the time we
# use the simpler add_user (below) for testing purposes.
function chasquid-util-user-add() {
	CONFDIR="${CONFDIR:-config}"
	DOMAIN=$(echo "$1" | cut -d @ -f 2)
	mkdir -p "${CONFDIR}/domains/$DOMAIN/"
	chasquid-util \
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
	go-build-cached "${TBASE}/../../cmd/dovecot-auth-cli/"
	"${TBASE}/../../cmd/dovecot-auth-cli/dovecot-auth-cli" "$@"
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

function mail_diff() {
	"${UTILDIR}/mail_diff" "$@"
}

function chamuyero() {
	"${UTILDIR}/chamuyero" "$@"
}

function generate_cert() {
	go-build-cached "${UTILDIR}/generate_cert/"
	"${UTILDIR}/generate_cert/generate_cert" "$@"
}

function loadgen() {
	go-build-cached "${UTILDIR}/loadgen/"
	"${UTILDIR}/loadgen/loadgen" "$@"
}

function conngen() {
	go-build-cached "${UTILDIR}/conngen/"
	"${UTILDIR}/conngen/conngen" "$@"
}

function minidns_bg() {
	go-build-cached "${UTILDIR}/minidns/"
	"${UTILDIR}/minidns/minidns" "$@" &
	export MINIDNS=$!
}

function fexp() {
	go-build-cached "${UTILDIR}/fexp/"
	"${UTILDIR}/fexp/fexp" "$@"
}

function smtpc() {
	go-build-cached "${UTILDIR}/smtpc/"
	"${UTILDIR}/smtpc/smtpc" "$@"
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
	mkdir -p "${CACHEDIR}/$1/"
	touch -d "10 minutes ago" "${CACHEDIR}/.reference"
	if [ "${CACHEDIR}/$1/privkey.pem" -ot "${CACHEDIR}/.reference" ]; then
		# Cache miss (either was not there, or was too old).
		(
			cd "${CACHEDIR}/$1/" || exit 1
			generate_cert -ca -validfor=1h -host="$1"
		)
	fi
	mkdir -p "${CONFDIR}/certs/$1/"
	cp -p "${CACHEDIR}/$1"/* "${CONFDIR}/certs/$1/"
}

function chasquid_ram_peak() {
	# Find the pid of the daemon, which we expect is running on the
	# background somewhere within our current session.
	SERVER_PID=$(pgrep -s 0 -x chasquid)
	grep VmHWM "/proc/$SERVER_PID/status" | cut -d ':' -f 2-
}
