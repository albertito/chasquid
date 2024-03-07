package safeio

import (
	"bytes"
	"errors"
	"fmt"
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
	c, err := os.ReadFile(fname)
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

type testFile struct {
	t *testing.T

	name string

	expectChmod os.FileMode
	chmodErr    error

	expectChownUid, expectChownGid int
	chownErr                       error

	expectWrite []byte
	writeN      int
	writeErr    error

	closeErr error
}

func (f *testFile) Name() string {
	return f.name
}

func (f *testFile) Chmod(perm os.FileMode) error {
	if f.expectChmod != perm {
		f.t.Errorf("unexpected Chmod(%v), expected Chmod(%v)",
			perm, f.expectChmod)
	}
	return f.chmodErr
}

func (f *testFile) Chown(uid, gid int) error {
	if f.expectChownUid != uid || f.expectChownGid != gid {
		f.t.Errorf("unexpected Chown(%v, %v), expected Chown(%v, %v)",
			uid, gid, f.expectChownUid, f.expectChownGid)
	}
	return f.chownErr
}

func (f *testFile) Write(b []byte) (int, error) {
	if !bytes.Equal(b, f.expectWrite) {
		f.t.Errorf("unexpected Write(%q), expected Write(%q)",
			b, f.expectWrite)
	}
	return f.writeN, f.writeErr
}

func (f *testFile) Close() error {
	return f.closeErr
}

var _ osFile = &testFile{}

func TestErrors(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	oldCreateTemp := createTemp
	defer func() { createTemp = oldCreateTemp }()

	// createTemp failure.
	ctError := errors.New("createTemp error")
	createTemp = func(dir, pattern string) (osFile, error) {
		return nil, ctError
	}
	err := WriteFile("fname", []byte("new content"), 0660)
	if err != ctError {
		t.Errorf("expected %v, got %v", ctError, err)
	}

	// Have a real backing file for some of the operations, like getting the
	// owner.
	fname := dir + "/file1"

	// Test file to simulate failures on.
	tf := &testFile{name: fname, t: t}
	createTemp = func(dir, pattern string) (osFile, error) {
		return tf, nil
	}

	// Test Chmod error.
	testlib.Rewrite(t, fname, "old content")
	tf.expectChmod = 0660
	tf.chmodErr = errors.New("chmod error")
	err = WriteFile(fname, []byte("new content"), 0660)
	if err != tf.chmodErr {
		t.Errorf("expected %v, got %v", tf.chmodErr, err)
	}
	checkNotExists(t, fname)

	// Test Chown error.
	testlib.Rewrite(t, fname, "old content")
	tf.chmodErr = nil
	tf.expectChownUid, tf.expectChownGid = getOwner(fname)
	if tf.expectChownUid < 0 {
		t.Fatalf("error getting owner of %v", fname)
	}
	tf.chownErr = errors.New("chown error")
	err = WriteFile(fname, []byte("new content"), 0660)
	if err != tf.chownErr {
		t.Errorf("expected %v, got %v", tf.chownErr, err)
	}
	checkNotExists(t, fname)

	// Test Write error.
	testlib.Rewrite(t, fname, "old content")
	tf.chownErr = nil
	tf.expectWrite = []byte("new content")
	tf.writeErr = errors.New("write error")
	err = WriteFile(fname, []byte("new content"), 0660)
	if err != tf.writeErr {
		t.Errorf("expected %v, got %v", tf.writeErr, err)
	}
	checkNotExists(t, fname)

	// Test Close error.
	testlib.Rewrite(t, fname, "old content")
	tf.writeErr = nil
	tf.writeN = len(tf.expectWrite)
	tf.closeErr = errors.New("close error")
	err = WriteFile(fname, []byte("new content"), 0660)
	if err != tf.closeErr {
		t.Errorf("expected %v, got %v", tf.closeErr, err)
	}
	checkNotExists(t, fname)
}

func checkNotExists(t *testing.T, fname string) {
	t.Helper()
	if _, err := os.Stat(fname); err == nil {
		t.Fatalf("file %v exists", fname)
	}
}
