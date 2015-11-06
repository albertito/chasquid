package courier

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestProcmail(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-chasquid-courier")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	procmailBin = "tee"
	procmailArgs = []string{dir + "/%user%"}

	p := Procmail{}
	err = p.Deliver("from@x", "to@y", []byte("data"))
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	data, err := ioutil.ReadFile(dir + "/to")
	if err != nil || !bytes.Equal(data, []byte("data")) {
		t.Errorf("Invalid data: %q - %v", string(data), err)
	}
}

func TestProcmailTimeout(t *testing.T) {
	procmailBin = "/bin/sleep"
	procmailArgs = []string{"1"}
	procmailTimeout = 100 * time.Millisecond

	p := Procmail{}
	err := p.Deliver("from", "to", []byte("data"))
	if err != timeoutError {
		t.Errorf("Unexpected error: %v", err)
	}

	procmailTimeout = 1 * time.Second
}

func TestProcmailBadCommandLine(t *testing.T) {
	p := Procmail{}

	// Non-existent binary.
	procmailBin = "thisdoesnotexist"
	err := p.Deliver("from", "to", []byte("data"))
	if err == nil {
		t.Errorf("Unexpected success: %q %v", procmailBin, procmailArgs)
	}

	// Incorrect arguments.
	procmailBin = "cat"
	procmailArgs = []string{"--fail_unknown_option"}

	err = p.Deliver("from", "to", []byte("data"))
	if err == nil {
		t.Errorf("Unexpected success: %q %v", procmailBin, procmailArgs)
	}
}
