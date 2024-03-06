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
. "$(dirname "$0")/util/lib.sh"

init

cd "${TBASE}/.."

# Recreate the coverage output directory, to avoid including stale results
# from previous runs.
rm -rf .coverage
mkdir -p .coverage/sh .coverage/go .coverage/all
export COVER_DIR="$PWD/.coverage"

# Normal go tests.
# shellcheck disable=SC2046
go test \
	-covermode=count -coverpkg=./... \
	$(go list ./... | grep -v -E 'chasquid/cmd/|chasquid/test') \
	-args -test.gocoverdir="${COVER_DIR}/go/"

# Integration tests.
# Will run in coverage mode due to $COVER_DIR being set.
GOCOVERDIR="${COVER_DIR}/sh" setsid -w ./test/run.sh

# dovecot tests are also coverage-aware.
echo "dovecot cli ..."
GOCOVERDIR="${COVER_DIR}/sh" setsid -w ./cmd/dovecot-auth-cli/test.sh

echo "chasquid-util ..."
GOCOVERDIR="${COVER_DIR}/sh" setsid -w ./cmd/chasquid-util/test.sh

# Merge all coverage output into a single file.
go tool covdata merge -i "${COVER_DIR}/go,${COVER_DIR}/sh" -o "${COVER_DIR}/all"
go tool covdata textfmt -i "${COVER_DIR}/all" -o "${COVER_DIR}/merged.out"

# Ignore protocol buffer-generated files, as they are not relevant.
grep -v ".pb.go:" < "${COVER_DIR}/merged.out" > "${COVER_DIR}/final.out"

# Generate reports based on the merged output.
go tool cover -func="$COVER_DIR/final.out" | sort -k 3 -n > "$COVER_DIR/func.txt"
go tool cover -html="$COVER_DIR/final.out" -o "$COVER_DIR/classic.html"
go run "${UTILDIR}/coverhtml/coverhtml.go" \
	-input="$COVER_DIR/final.out"  -strip=3 \
	-output="$COVER_DIR/coverage.html" \
	-title="chasquid coverage report" \
	-notes="Generated at commit <tt>$(git describe --always --dirty --tags)</tt> ($(git log -1 --format=%ci))"

echo
echo
echo "Coverage report can be found in:"
echo "file://$COVER_DIR/coverage.html"

