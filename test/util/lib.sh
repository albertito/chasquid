# Library to write the shell scripts in the tests.

function init() {
	if [ "$V" == "1" ]; then
		set -v
	fi

	export UTILDIR="$( realpath `dirname "${BASH_SOURCE[0]}"` )"

	export TBASE="$(realpath `dirname ${0}`)"
	cd ${TBASE}

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
	if [ "${COVER_DIR}" != "" ]; then
		chasquid_cover "$@"
		return
	fi

	( cd ${TBASE}/../../; go build $GOFLAGS -tags="$GOTAGS" . )

	# HOSTALIASES: so we "fake" hostnames.
	# PATH: so chasquid can call test-mda without path issues.
	# MDA_DIR: so our test-mda knows where to deliver emails.
	HOSTALIASES=${TBASE}/hosts \
	PATH=${UTILDIR}:${PATH} \
	MDA_DIR=${TBASE}/.mail \
		${TBASE}/../../chasquid "$@"
}

function chasquid_cover() {
	# Build the coverage-enabled binary.
	# See coverage_test.go for more details.
	( cd ${TBASE}/../../;
	  go test -covermode=count -coverpkg=./... -c \
		  -tags="coveragebin $GOTAGS" $GOFLAGS )

	# Run the coverage-enabled binary, named "chasquid.test" for hacky
	# reasons.  See the chasquid function above for details on the
	# environment variables.
	HOSTALIASES=${TBASE}/hosts \
	PATH=${UTILDIR}:${PATH} \
	MDA_DIR=${TBASE}/.mail \
		${TBASE}/../../chasquid.test \
			-test.run "^TestRunMain$" \
			-test.coverprofile="$COVER_DIR/test-`date +%s.%N`.out" \
			"$@"
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

function dovecot-auth-cli() {
	go run ${TBASE}/../../cmd/dovecot-auth-cli/dovecot-auth-cli.go "$@"
}

function run_msmtp() {
	# msmtp will check that the rc file is only user readable.
	chmod 600 msmtprc

	# msmtp binary is often g+s, which causes $HOSTALIASES to not be
	# honoured, which breaks the tests. Copy the binary to remove the
	# setgid bit as a workaround.
	cp -u "`which msmtp`" "${UTILDIR}/.msmtp-bin"

	HOSTALIASES=${TBASE}/hosts \
		${UTILDIR}/.msmtp-bin -C msmtprc "$@"
}

function smtpc.py() {
	${UTILDIR}/smtpc.py "$@"
}

function mail_diff() {
	${UTILDIR}/mail_diff "$@"
}

function chamuyero() {
	${UTILDIR}/chamuyero "$@"
}

function generate_cert() {
	go run ${UTILDIR}/generate_cert.go "$@"
}

function loadgen() {
	go run ${UTILDIR}/loadgen.go "$@"
}

function conngen() {
	go run ${UTILDIR}/conngen.go "$@"
}

function minidns_bg() {
	( cd ${UTILDIR}; go build minidns.go )
	${UTILDIR}/minidns "$@" &
	MINIDNS=$!
}

function fexp() {
	( cd ${UTILDIR}; go build fexp.go )
	${UTILDIR}/fexp "$@"
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

	while ! bash -c "true < /dev/tcp/localhost/$PORT" 2>/dev/null ; do
		sleep 0.1
	done
}

# Wait for the given file to exist.
function wait_for_file() {
	while ! [ -e ${1} ]; do
		sleep 0.1
	done
}

function wait_until() {
	while true; do
		if eval "$@"; then
			return 0
		fi
		sleep 0.05
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

function chasquid_ram_peak() {
	# Find the pid of the daemon, which we expect is running on the
	# background somewhere within our current session.
	SERVER_PID=`pgrep -s 0 -x chasquid`

	echo $( cat /proc/$SERVER_PID/status | grep VmHWM | cut -d ':' -f 2- )
}
