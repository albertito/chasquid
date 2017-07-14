package safeio

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
)

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
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

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
}

func TestWriteFileWithOp(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

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
}

func TestWriteFileWithFailingOp(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

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
}

// TODO: We should test the possible failure scenarios for WriteFile, but it
// gets tricky without being able to do failure injection (or turning the code
// into a mess).
