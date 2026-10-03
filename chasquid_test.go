package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
)

func mustMkdir(t *testing.T, path string) string {
	t.Helper()
	err := os.Mkdir(path, 0700)
	if err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	return path
}

func TestFindCertKey(t *testing.T) {
	tmpDir := t.ArtifactDir()

	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"certbot", []string{"privkey.pem"}, "privkey.pem"},
		{"lego", []string{"key.pem"}, "key.pem"},
		{"both", []string{"privkey.pem", "key.pem"}, "privkey.pem"},
	}
	for _, c := range cases {
		dir := mustMkdir(t, filepath.Join(tmpDir, c.name))
		for _, f := range c.files {
			testlib.Rewrite(t, filepath.Join(dir, f), "")
		}

		got, err := findCertKey(dir)
		if err != nil {
			t.Errorf("%s: unexpected error: %v", c.name, err)
		}
		if want := filepath.Join(dir, c.want); got != want {
			t.Errorf("%s: got %q, expected %q", c.name, got, want)
		}
	}

	// No key files: the error should mention the directory.
	dir := mustMkdir(t, filepath.Join(tmpDir, "empty"))
	got, err := findCertKey(dir)
	if err == nil || !strings.Contains(err.Error(), dir) {
		t.Errorf("empty: got %q, %v, expected 'no private key' error",
			got, err)
	}

	// privkey.pem is there but can't be stat'ed (a symlink loop here, which
	// also works when running as root). We expect that error to be returned,
	// instead of silently falling back to key.pem.
	dir = mustMkdir(t, filepath.Join(tmpDir, "loop"))
	err = os.Symlink("privkey.pem", filepath.Join(dir, "privkey.pem"))
	if err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}
	testlib.Rewrite(t, filepath.Join(dir, "key.pem"), "")
	got, err = findCertKey(dir)
	if !errors.Is(err, syscall.ELOOP) {
		t.Errorf("loop: got %q, %v, expected ELOOP error", got, err)
	}
}
