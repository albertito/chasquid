package config

import (
	"io"
	"io/ioutil"
	"os"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/log"
)

func mustCreateConfig(t *testing.T, contents string) (string, string) {
	tmpDir := testlib.MustTempDir(t)
	confStr := []byte(contents)
	err := ioutil.WriteFile(tmpDir+"/chasquid.conf", confStr, 0600)
	if err != nil {
		t.Fatalf("Failed to write tmp config: %v", err)
	}

	return tmpDir, tmpDir + "/chasquid.conf"
}

func TestEmptyConfig(t *testing.T) {
	tmpDir, path := mustCreateConfig(t, "")
	defer testlib.RemoveIfOk(t, tmpDir)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("error loading empty config: %v", err)
	}

	// Test the default values are set.

	hostname, _ := os.Hostname()
	if c.Hostname == "" || c.Hostname != hostname {
		t.Errorf("invalid hostname %q, should be: %q", c.Hostname, hostname)
	}

	if c.MaxDataSizeMb != 50 {
		t.Errorf("max data size != 50: %d", c.MaxDataSizeMb)
	}

	if len(c.SmtpAddress) != 1 || c.SmtpAddress[0] != "systemd" {
		t.Errorf("unexpected address default: %v", c.SmtpAddress)
	}

	if len(c.SubmissionAddress) != 1 || c.SubmissionAddress[0] != "systemd" {
		t.Errorf("unexpected address default: %v", c.SubmissionAddress)
	}

	if c.MonitoringAddress != "" {
		t.Errorf("monitoring address is set: %v", c.MonitoringAddress)
	}

	testLogConfig(c)
}

func TestFullConfig(t *testing.T) {
	confStr := `
		hostname: "joust"
		smtp_address: ":1234"
		smtp_address: ":5678"
		monitoring_address: ":1111"
		max_data_size_mb: 26
	`

	tmpDir, path := mustCreateConfig(t, confStr)
	defer testlib.RemoveIfOk(t, tmpDir)

	c, err := Load(path)
	if err != nil {
		t.Fatalf("error loading non-existent config: %v", err)
	}

	if c.Hostname != "joust" {
		t.Errorf("hostname %q != 'joust'", c.Hostname)
	}

	if c.MaxDataSizeMb != 26 {
		t.Errorf("max data size != 26: %d", c.MaxDataSizeMb)
	}

	if len(c.SmtpAddress) != 2 ||
		c.SmtpAddress[0] != ":1234" || c.SmtpAddress[1] != ":5678" {
		t.Errorf("different address: %v", c.SmtpAddress)
	}

	if c.MonitoringAddress != ":1111" {
		t.Errorf("monitoring address %q != ':1111;", c.MonitoringAddress)
	}

	testLogConfig(c)
}

func TestErrorLoading(t *testing.T) {
	c, err := Load("/does/not/exist")
	if err == nil {
		t.Fatalf("loaded a non-existent config: %v", c)
	}
}

func TestBrokenConfig(t *testing.T) {
	tmpDir, path := mustCreateConfig(
		t, "<invalid> this is not a valid protobuf")
	defer testlib.RemoveIfOk(t, tmpDir)

	c, err := Load(path)
	if err == nil {
		t.Fatalf("loaded an invalid config: %v", c)
	}
}

// Run LogConfig, overriding the default logger first. This exercises the
// code, we don't yet validate the output, but it is an useful sanity check.
func testLogConfig(c *Config) {
	l := log.New(nopWCloser{ioutil.Discard})
	log.Default = l
	LogConfig(c)
}

type nopWCloser struct {
	io.Writer
}

func (nopWCloser) Close() error { return nil }
