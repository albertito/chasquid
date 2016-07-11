package safeio

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
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

func testWriteFile(fname string, data []byte, perm os.FileMode) error {
	err := WriteFile("file1", data, perm)
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

// TODO: We should test the possible failure scenarios for WriteFile, but it
// gets tricky without being able to do failure injection (or turning the code
// into a mess).
