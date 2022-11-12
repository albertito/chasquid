package config

import (
	"io"
	"os"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/testlib"
	"blitiri.com.ar/go/log"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func mustCreateConfig(t *testing.T, contents string) (string, string) {
	tmpDir := testlib.MustTempDir(t)
	confStr := []byte(contents)
	err := os.WriteFile(tmpDir+"/chasquid.conf", confStr, 0600)
	if err != nil {
		t.Fatalf("Failed to write tmp config: %v", err)
	}

	return tmpDir, tmpDir + "/chasquid.conf"
}

func TestEmptyStruct(t *testing.T) {
	testLogConfig(&Config{})
}

func TestEmptyConfig(t *testing.T) {
	tmpDir, path := mustCreateConfig(t, "")
	defer testlib.RemoveIfOk(t, tmpDir)
	c, err := Load(path, "")
	if err != nil {
		t.Fatalf("error loading empty config: %v", err)
	}

	// Test the default values are set.
	defaults := proto.Clone(defaultConfig).(*Config)
	hostname, _ := os.Hostname()
	defaults.Hostname = hostname

	diff := cmp.Diff(defaults, c, protocmp.Transform())
	if diff != "" {
		t.Errorf("Load() mismatch (-want +got):\n%s", diff)
	}

	testLogConfig(c)
}

func TestFullConfig(t *testing.T) {
	confStr := `
		hostname: "joust"
		smtp_address: ":1234"
		smtp_address: ":5678"
		submission_address: ":10001"
		submission_address: ":10002"
		monitoring_address: ":1111"
		max_data_size_mb: 26
		suffix_separators: ""
	`

	tmpDir, path := mustCreateConfig(t, confStr)
	defer testlib.RemoveIfOk(t, tmpDir)

	overrideStr := `
		hostname: "proust"
		submission_address: ":999"
		dovecot_auth: true
		drop_characters: ""
	`

	expected := &Config{
		Hostname:      "proust",
		MaxDataSizeMb: 26,

		SmtpAddress:              []string{":1234", ":5678"},
		SubmissionAddress:        []string{":999"},
		SubmissionOverTlsAddress: []string{"systemd"},
		MonitoringAddress:        ":1111",

		MailDeliveryAgentBin:  "maildrop",
		MailDeliveryAgentArgs: []string{"-f", "%from%", "-d", "%to_user%"},

		DataDir: "/var/lib/chasquid",

		SuffixSeparators: proto.String(""),
		DropCharacters:   proto.String(""),

		MailLogPath: "<syslog>",

		DovecotAuth: true,
	}

	c, err := Load(path, overrideStr)
	if err != nil {
		t.Fatalf("error loading non-existent config: %v", err)
	}

	diff := cmp.Diff(expected, c, protocmp.Transform())
	if diff != "" {
		t.Errorf("Load() mismatch (-want +got):\n%s", diff)
	}

	testLogConfig(c)
}

func TestErrorLoading(t *testing.T) {
	c, err := Load("/does/not/exist", "")
	if err == nil {
		t.Fatalf("loaded a non-existent config: %v", c)
	}
}

func TestBrokenConfig(t *testing.T) {
	tmpDir, path := mustCreateConfig(
		t, "<invalid> this is not a valid protobuf")
	defer testlib.RemoveIfOk(t, tmpDir)

	c, err := Load(path, "")
	if err == nil {
		t.Fatalf("loaded an invalid config: %v", c)
	}
}

func TestBrokenOverride(t *testing.T) {
	tmpDir, path := mustCreateConfig(
		t, `hostname: "test"`)
	defer testlib.RemoveIfOk(t, tmpDir)

	c, err := Load(path, "broken override")
	if err == nil {
		t.Fatalf("loaded an invalid config: %v", c)
	}
}

// Run LogConfig, overriding the default logger first. This exercises the
// code, we don't yet validate the output, but it is an useful sanity check.
func testLogConfig(c *Config) {
	l := log.New(nopWCloser{io.Discard})
	log.Default = l
	LogConfig(c)
}

type nopWCloser struct {
	io.Writer
}

func (nopWCloser) Close() error { return nil }
