package testlib

import (
	"os"
	"testing"
)

func TestRewrite(t *testing.T) {
	fname := t.TempDir() + "/file"
	Rewrite(t, fname, "hola")
	Rewrite(t, fname, "chau")

	data, err := os.ReadFile(fname)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(data) != "chau" {
		t.Errorf("file content: got %q, expected %q", data, "chau")
	}
}

func TestGenerateCert(t *testing.T) {
	conf, err := GenerateCert(t.TempDir())
	if err != nil {
		t.Errorf("GenerateCert returned error: %v", err)
	}
	if conf.ServerName != "localhost" {
		t.Errorf("Config server name %q != localhost", conf.ServerName)
	}
	if conf.RootCAs == nil {
		t.Errorf("Config had an empty RootCAs pool")
	}
}

func TestGenerateCertBadDir(t *testing.T) {
	conf, err := GenerateCert("/doesnotexist/")
	if err == nil || conf != nil {
		t.Fatalf("GenerateCert returned non-error: %v / %v", conf, err)
	}
}
