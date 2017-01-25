package safeio

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

func mustTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "safeio_test")
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

func testWriteFile(fname string, data []byte, perm os.FileMode, ops ...FileOp) error {
	err := WriteFile("file1", data, perm, ops...)
	if err != nil {
		return fmt.Errorf("error writing new file: %v", err)
	}

	// Read and compare the contents.
	c, err := ioutil.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("error reading: %v", err)
	}

	if !bytes.Equal(data, c) {
		return fmt.Errorf("expected %q, got %q", data, c)
	}

	// Check permissions.
	st, err := os.Stat("file1")
	if err != nil {
		return fmt.Errorf("error in stat: %v", err)
	}
	if st.Mode() != perm {
		return fmt.Errorf("permissions mismatch, expected %#o, got %#o",
			st.Mode(), perm)
	}

	return nil
}

func TestWriteFile(t *testing.T) {
	dir := mustTempDir(t)

	// Write a new file.
	content := []byte("content 1")
	if err := testWriteFile("file1", content, 0660); err != nil {
		t.Error(err)
	}

	// Write an existing file.
	content = []byte("content 2")
	if err := testWriteFile("file1", content, 0660); err != nil {
		t.Error(err)
	}

	// Write again, but this time change permissions.
	content = []byte("content 3")
	if err := testWriteFile("file1", content, 0600); err != nil {
		t.Error(err)
	}

	// Remove the test directory, but only if we have not failed. We want to
	// keep the failed structure for debugging.
	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestWriteFileWithOp(t *testing.T) {
	dir := mustTempDir(t)

	var opFile string
	op := func(f string) error {
		opFile = f
		return nil
	}

	content := []byte("content 1")
	if err := testWriteFile("file1", content, 0660, op); err != nil {
		t.Error(err)
	}

	if opFile == "" {
		t.Error("operation was not called")
	}
	if !strings.Contains(opFile, "file1") {
		t.Errorf("operation called with suspicious file: %s", opFile)
	}

	// Remove the test directory, but only if we have not failed. We want to
	// keep the failed structure for debugging.
	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestWriteFileWithFailingOp(t *testing.T) {
	dir := mustTempDir(t)

	var opFile string
	opOK := func(f string) error {
		opFile = f
		return nil
	}

	opError := errors.New("operation failed")
	opFail := func(f string) error {
		return opError
	}

	content := []byte("content 1")
	err := WriteFile("file1", content, 0660, opOK, opOK, opFail)
	if err != opError {
		t.Errorf("different error, got %v, expected %v", err, opError)
	}

	if _, err := os.Stat(opFile); err == nil {
		t.Errorf("temporary file was not removed after failure (%v)", opFile)
	}

	// Remove the test directory, but only if we have not failed. We want to
	// keep the failed structure for debugging.
	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

// TODO: We should test the possible failure scenarios for WriteFile, but it
// gets tricky without being able to do failure injection (or turning the code
// into a mess).
