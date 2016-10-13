package smtpsrv

import (
	"io/ioutil"
	"os"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/spf"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

func TestSecLevel(t *testing.T) {
	// We can't simulate this externally because of the SPF record
	// requirement, so do a narrow test on Conn.secLevelCheck.
	tmpDir, err := ioutil.TempDir("", "chasquid_test:")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dinfo, err := domaininfo.New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create domain info: %v", err)
	}

	c := &Conn{
		tr:    trace.New("testconn", "testconn"),
		dinfo: dinfo,
	}

	// No SPF, skip security checks.
	c.spfResult = spf.None
	c.onTLS = true
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("TLS seclevel failed")
	}

	c.onTLS = false
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel failed, even though SPF does not exist")
	}

	// Now the real checks, once SPF passes.
	c.spfResult = spf.Pass

	if !c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel failed")
	}

	c.onTLS = true
	if !c.secLevelCheck("from@slc") {
		t.Fatalf("TLS seclevel failed")
	}

	c.onTLS = false
	if c.secLevelCheck("from@slc") {
		t.Fatalf("plain seclevel worked, downgrade was allowed")
	}
}
