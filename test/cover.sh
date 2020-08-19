#!/bin/bash

# Runs tests (both go and integration) in coverage-generation mode.
# Generates an HTML report with the results.
#
# The .coverage directory is used to store the data, it will be erased and
# recreated on each run.
#
# This is not very tidy, and relies on some hacky tricks (see
# coverage_test.go), but works for now.

set -e
. $(dirname ${0})/util/lib.sh

init

cd "${TBASE}/.."

# Recreate the coverage output directory, to avoid including stale results
# from previous runs.
rm -rf .coverage
mkdir -p .coverage
export COVER_DIR="$PWD/.coverage"

# Normal go tests.
# We have to run them one by one because the expvar registration causes
# the single-binary tests to fail: cross-package expvars confuse the expvarom
# tests, which don't expect any expvars to exists besides the one registered
# in the tests themselves.
for pkg in $(go list ./... | grep -v chasquid/cmd/); do
	OUT_FILE="$COVER_DIR/pkg-`echo $pkg | sed s+/+_+g`.out"
	go test -tags coverage \
		-covermode=count \
		-coverprofile="$OUT_FILE" \
		-coverpkg=./... $pkg
done

# Integration tests.
# Will run in coverage mode due to $COVER_DIR being set.
setsid -w ./test/run.sh

# dovecot tests are also coverage-aware.
echo "dovecot cli ..."
setsid -w ./cmd/dovecot-auth-cli/test.sh

# Merge all coverage output into a single file.
# Ignore protocol buffer-generated files, as they are not relevant.
go run "${UTILDIR}/gocovcat.go" .coverage/*.out \
	| grep -v ".pb.go:" \
	> .coverage/all.out

# Generate reports based on the merged output.
go tool cover -func="$COVER_DIR/all.out" | sort -k 3 -n > "$COVER_DIR/func.txt"
go tool cover -html="$COVER_DIR/all.out" -o "$COVER_DIR/classic.html"
go run "${UTILDIR}/coverhtml.go" \
	-input="$COVER_DIR/all.out"  -strip=3 \
	-output="$COVER_DIR/coverage.html" \
	-title="chasquid coverage report" \
	-notes="Generated at commit <tt>$(git describe --always --dirty)</tt> ($(git log -1 --format=%ci))"

echo
echo
echo "Coverage report can be found in:"
echo file://$COVER_DIR/coverage.html

