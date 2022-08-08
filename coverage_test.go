// Test file used to build a coverage-enabled chasquid binary.
//
// Go lacks support for properly building a coverage binary, it can only build
// coverage test binaries.  As a workaround, we have a test that just runs
// main. We then build a binary of this test, which we use instead of chasquid
// in integration tests.
//
// This is hacky and horrible.
//
// The test has a build label so it's not accidentally executed during normal
// "go test" invocations.
//go:build coveragebin
// +build coveragebin

package main

import (
	"os"
	"os/signal"
	"syscall"
	"testing"
)

func TestRunMain(t *testing.T) {
	done := make(chan bool)

	signals := make(chan os.Signal, 1)
	go func() {
		<-signals
		done <- true
	}()
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGTERM)

	go func() {
		main()
		done <- true
	}()

	<-done
}
