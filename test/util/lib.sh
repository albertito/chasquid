# Library to write the shell scripts in the tests.

function init() {
	if [ "$V" == "1" ]; then
		set -v
	fi

	export TBASE="$(realpath `dirname ${0}`)"
	cd ${TBASE}

	export UTILDIR="$(realpath ${TBASE}/../util/)"

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
		go run ${TBASE}/../../chasquid.go "$@"
}

function add_user() {
	go run ${TBASE}/../../cmd/chasquid-userdb/chasquid-userdb.go \
		--database "config/domains/${1}/users" \
		--add_user "${2}" \
		--password "${3}" \
		>> .add_user_logs
}

function run_msmtp() {
	# msmtp will check that the rc file is only user readable.
	chmod 600 msmtprc

	HOSTALIASES=${TBASE}/hosts \
		msmtp -C msmtprc "$@"
}

function mail_diff() {
	${UTILDIR}/mail_diff "$@"
}

function success() {
	echo "SUCCESS"
}

# Wait until there's something listening on the given port.
function wait_until_ready() {
	PORT=$1

	while ! nc -z localhost $PORT; do
		sleep 0.1
	done
}

# Wait for the given file to exist.
function wait_for_file() {
	while ! [ -e ${1} ]; do
		sleep 0.1
	done
}

# Generate certs for the given domain.
function generate_certs_for() {
	mkdir -p config/domains/${1}
	(
		cd config/domains/${1}
		generate_cert -ca -duration=1h -host=${1}
	)
}
