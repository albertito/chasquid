package spf

import (
	"flag"
	"fmt"
	"net"
	"os"
	"testing"
)

var txtResults = map[string][]string{}
var txtErrors = map[string]error{}

func LookupTXT(domain string) (txts []string, err error) {
	return txtResults[domain], txtErrors[domain]
}

var mxResults = map[string][]*net.MX{}
var mxErrors = map[string]error{}

func LookupMX(domain string) (mxs []*net.MX, err error) {
	return mxResults[domain], mxErrors[domain]
}

var ipResults = map[string][]net.IP{}
var ipErrors = map[string]error{}

func LookupIP(host string) (ips []net.IP, err error) {
	return ipResults[host], ipErrors[host]
}

func TestMain(m *testing.M) {
	lookupTXT = LookupTXT
	lookupMX = LookupMX
	lookupIP = LookupIP

	flag.Parse()
	os.Exit(m.Run())
}

var ip1110 = net.ParseIP("1.1.1.0")
var ip1111 = net.ParseIP("1.1.1.1")
var ip6666 = net.ParseIP("2001:db8::68")

func TestBasic(t *testing.T) {
	cases := []struct {
		txt string
		res Result
	}{
		{"", None},
		{"blah", None},
		{"v=spf1", Neutral},
		{"v=spf1 ", Neutral},
		{"v=spf1 -", PermError},
		{"v=spf1 all", Pass},
		{"v=spf1  +all", Pass},
		{"v=spf1 -all ", Fail},
		{"v=spf1 ~all", SoftFail},
		{"v=spf1 ?all", Neutral},
		{"v=spf1 a ~all", SoftFail},
		{"v=spf1 a/24", Neutral},
		{"v=spf1 a:d1110/24", Pass},
		{"v=spf1 a:d1110", Neutral},
		{"v=spf1 a:d1111", Pass},
		{"v=spf1 a:nothing/24", Neutral},
		{"v=spf1 mx", Neutral},
		{"v=spf1 mx/24", Neutral},
		{"v=spf1 mx:a/montoto ~all", PermError},
		{"v=spf1 mx:d1110/24 ~all", Pass},
		{"v=spf1 ip4:1.2.3.4 ~all", SoftFail},
		{"v=spf1 ip6:12 ~all", PermError},
		{"v=spf1 ip4:1.1.1.1 -all", Pass},
		{"v=spf1 blah", PermError},
	}

	ipResults["d1111"] = []net.IP{ip1111}
	ipResults["d1110"] = []net.IP{ip1110}
	mxResults["d1110"] = []*net.MX{{"d1110", 5}, {"nothing", 10}}

	for _, c := range cases {
		txtResults["domain"] = []string{c.txt}
		res, err := CheckHost(ip1111, "domain")
		if (res == TempError || res == PermError) && (err == nil) {
			t.Errorf("%q: expected error, got nil", c.txt)
		}
		if res != c.res {
			t.Errorf("%q: expected %q, got %q", c.txt, c.res, res)
			t.Logf("%q:   error: %v", c.txt, err)
		}
	}
}

func TestNotSupported(t *testing.T) {
	cases := []string{
		"v=spf1 exists:blah -all",
		"v=spf1 ptr -all",
		"v=spf1 exp=blah -all",
		"v=spf1 a:%{o} -all",
	}

	for _, txt := range cases {
		txtResults["domain"] = []string{txt}
		res, err := CheckHost(ip1111, "domain")
		if res != Neutral {
			t.Errorf("%q: expected neutral, got %v", txt, res)
			t.Logf("%q:   error: %v", txt, err)
		}
	}
}

func TestRecursion(t *testing.T) {
	txtResults["domain"] = []string{"v=spf1 include:domain ~all"}

	res, err := CheckHost(ip1111, "domain")
	if res != PermError {
		t.Errorf("expected permerror, got %v (%v)", res, err)
	}
}

func TestNoRecord(t *testing.T) {
	txtResults["d1"] = []string{""}
	txtResults["d2"] = []string{"loco", "v=spf2"}
	txtErrors["nospf"] = fmt.Errorf("no such domain")

	for _, domain := range []string{"d1", "d2", "d3", "nospf"} {
		res, err := CheckHost(ip1111, domain)
		if res != None {
			t.Errorf("expected none, got %v (%v)", res, err)
		}
	}
}
