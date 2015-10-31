package config

import (
	"io/ioutil"
	"os"
	"testing"
)

func mustCreateConfig(t *testing.T, contents string) (string, string) {
	tmpDir, err := ioutil.TempDir("", "chasquid_config_test:")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v\n", tmpDir)
	}

	confStr := []byte(contents)
	err = ioutil.WriteFile(tmpDir+"/chasquid.conf", confStr, 0600)
	if err != nil {
		t.Fatalf("Failed to write tmp config: %v", err)
	}

	return tmpDir, tmpDir + "/chasquid.conf"
}

func TestEmptyConfig(t *testing.T) {
	tmpDir, path := mustCreateConfig(t, "")
	defer os.RemoveAll(tmpDir)
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

	if len(c.Address) != 1 || c.Address[0] != "systemd" {
		t.Errorf("unexpected address default: %v", c.Address)
	}

	if c.MonitoringAddress != "" {
		t.Errorf("monitoring address is set: %v", c.MonitoringAddress)
	}

}

func TestFullConfig(t *testing.T) {
	confStr := `
		hostname: "joust"
		address: ":1234"
		address: ":5678"
		monitoring_address: ":1111"
		max_data_size_mb: 26
	`

	tmpDir, path := mustCreateConfig(t, confStr)
	defer os.RemoveAll(tmpDir)

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

	if len(c.Address) != 2 ||
		c.Address[0] != ":1234" || c.Address[1] != ":5678" {
		t.Errorf("different address: %v", c.Address)
	}

	if c.MonitoringAddress != ":1111" {
		t.Errorf("monitoring address %q != ':1111;", c.MonitoringAddress)
	}
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
	defer os.RemoveAll(tmpDir)

	c, err := Load(path)
	if err == nil {
		t.Fatalf("loaded an invalid config: %v", c)
	}
}
