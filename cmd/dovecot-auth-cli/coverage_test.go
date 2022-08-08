// This package is tested externally (see test.sh).
// However, we need this to do coverage tests.
//
// See coverage_test.go for the details, this is the same horrible hack.
//
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
