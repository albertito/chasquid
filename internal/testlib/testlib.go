// Package testlib provides common test utilities.
package testlib

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

// MustTempDir creates a temporary directory, or dies trying.
func MustTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "testlib_")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chdir(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("test directory: %q", dir)
	return dir
}

// RemoveIfOk removes the given directory, but only if we have not failed. We
// want to keep the failed directories for debugging.
func RemoveIfOk(t *testing.T, dir string) {
	// Safeguard, to make sure we only remove test directories.
	// This should help prevent accidental deletions.
	if !strings.Contains(dir, "testlib_") {
		panic("invalid/dangerous directory")
	}

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}
