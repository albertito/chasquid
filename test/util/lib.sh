# Library to write the shell scripts in the tests.

function init() {
	if [ "$V" == "1" ]; then
		set -v
	fi

	export UTILDIR="$( realpath `dirname "${BASH_SOURCE[0]}"` )"

	export TBASE="$(realpath `dirname ${0}`)"
	cd ${TBASE}

	if [ "${RACE}" == "1" ]; then
		RACE="-race"
	fi

	# Remove the directory where test-mda will deliver mail, so previous
	# runs don't interfere with this one.
	rm -rf .mail

	# Set traps to kill our subprocesses when we exit (for any reason).
	# https://stackoverflow.com/questions/360201/
	trap "exit" INT TERM
	trap "kill 0" EXIT
}

function generate_cert() {
	go run ${UTILDIR}/generate_cert.go "$@"
}

function chasquid() {
	# HOSTALIASES: so we "fake" hostnames.
	# PATH: so chasquid can call test-mda without path issues.
	# MDA_DIR: so our test-mda knows where to deliver emails.
	HOSTALIASES=${TBASE}/hosts \
	PATH=${UTILDIR}:${PATH} \
	MDA_DIR=${TBASE}/.mail \
		go run ${RACE} ${TBASE}/../../chasquid.go "$@"
}

function add_user() {
	CONFDIR="${CONFDIR:-config}"
	DOMAIN=$(echo $1 | cut -d @ -f 2)
	mkdir -p "${CONFDIR}/domains/$DOMAIN/"
	go run ${TBASE}/../../cmd/chasquid-util/chasquid-util.go \
		-C "${CONFDIR}" \
		user-add "$1" \
		--password "$2" \
		>> .add_user_logs
}

function run_msmtp() {
	# msmtp will check that the rc file is only user readable.
	chmod 600 msmtprc

	HOSTALIASES=${TBASE}/hosts \
		msmtp -C msmtprc "$@"
}

function smtpc.py() {
	${UTILDIR}/smtpc.py "$@"
}

function nc.py() {
	${UTILDIR}/nc.py "$@"
}

function mail_diff() {
	${UTILDIR}/mail_diff "$@"
}

function chamuyero() {
	${UTILDIR}/chamuyero "$@"
}

function success() {
	echo success
}

function skip() {
	echo skipped: $*
	exit 0
}

function fail() {
	echo FAILED: $*
	exit 1
}

# Wait until there's something listening on the given port.
function wait_until_ready() {
	PORT=$1

	while ! nc.py -z localhost $PORT; do
		sleep 0.1
	done
}

# Wait for the given file to exist.
function wait_for_file() {
	while ! [ -e ${1} ]; do
		sleep 0.1
	done
}

# Generate certs for the given hostname.
function generate_certs_for() {
	CONFDIR="${CONFDIR:-config}"
	mkdir -p ${CONFDIR}/certs/${1}/
	(
		cd ${CONFDIR}/certs/${1}
		generate_cert -ca -duration=1h -host=${1}
	)
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
