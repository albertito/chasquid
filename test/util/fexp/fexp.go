//go:build !coverage
// +build !coverage

// Fetch an URL, and check if the response matches what we expect.
//
// Useful for testing HTTP(s) servers.
package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var exitCode int

func main() {
	if len(os.Args) < 2 {
		fatalf("Usage: fexp <URL> <options>\n")
	}

	// The first arg is the URL, and then we shift.
	url := os.Args[1]
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	var (
		body = flag.String("body", "",
			"expect body with these exact contents")
		bodyRE = flag.String("bodyre", "",
			"expect body matching these contents (regexp match)")
		bodyNotRE = flag.String("bodynotre", "",
			"expect body NOT matching these contents (regexp match)")
		redir = flag.String("redir", "",
			"expect a redirect to this URL")
		status = flag.Int("status", 200,
			"expect this status code")
		verbose = flag.Bool("v", false,
			"enable verbose output")
		save = flag.String("save", "",
			"save body to this file")
		method = flag.String("method", "GET",
			"request method to use")
		hdrRE = flag.String("hdrre", "",
			"expect a header matching these contents (regexp match)")
		caCert = flag.String("cacert", "",
			"file to read CA cert from")
	)
	flag.Parse()

	client := &http.Client{
		CheckRedirect: noRedirect,
		Transport:     mkTransport(*caCert),
	}

	req, err := http.NewRequest(*method, url, nil)
	if err != nil {
		fatalf("error building request: %q", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		fatalf("error getting %q: %v\n", url, err)
	}
	defer resp.Body.Close()
	rbody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		errorf("error reading body: %v\n", err)
	}

	if *save != "" {
		err = ioutil.WriteFile(*save, rbody, 0664)
		if err != nil {
			errorf("error writing body to file %q: %v\n", *save, err)
		}
	}

	if *verbose {
		fmt.Printf("Request: %s\n", url)
		fmt.Printf("Response:\n")
		fmt.Printf("  %v  %v\n", resp.Proto, resp.Status)
		ks := []string{}
		for k := range resp.Header {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			fmt.Printf("  %v: %s\n", k,
				strings.Join(resp.Header[k], ", "))
		}
		fmt.Printf("\n")
	}

	if resp.StatusCode != *status {
		errorf("status is not %d: %q\n", *status, resp.Status)
	}

	if *body != "" {
		// Unescape the body to allow control characters more easily.
		*body, _ = strconv.Unquote("\"" + *body + "\"")
		if string(rbody) != *body {
			errorf("unexpected body: %q\n", rbody)
		}
	}

	if *bodyRE != "" {
		matched, err := regexp.Match(*bodyRE, rbody)
		if err != nil {
			errorf("regexp error: %q\n", err)
		}
		if !matched {
			errorf("body did not match regexp: %q\n", rbody)
		}
	}

	if *bodyNotRE != "" {
		matched, err := regexp.Match(*bodyNotRE, rbody)
		if err != nil {
			errorf("regexp error: %q\n", err)
		}
		if matched {
			errorf("body matched regexp: %q\n", rbody)
		}
	}

	if *redir != "" {
		if loc := resp.Header.Get("Location"); loc != *redir {
			errorf("unexpected redir location: %q\n", loc)
		}
	}

	if *hdrRE != "" {
		match := false
	outer:
		for k, vs := range resp.Header {
			for _, v := range vs {
				hdr := fmt.Sprintf("%s: %s", k, v)
				matched, err := regexp.MatchString(*hdrRE, hdr)
				if err != nil {
					errorf("regexp error: %q\n", err)
				}
				if matched {
					match = true
					break outer
				}
			}
		}

		if !match {
			errorf("header did not match: %v\n", resp.Header)
		}
	}

	os.Exit(exitCode)
}

func noRedirect(req *http.Request, via []*http.Request) error {
	return http.ErrUseLastResponse
}

func mkTransport(caCert string) http.RoundTripper {
	if caCert == "" {
		return nil
	}

	certs, err := ioutil.ReadFile(caCert)
	if err != nil {
		fatalf("error reading CA file %q: %v\n", caCert, err)
	}

	rootCAs := x509.NewCertPool()
	if ok := rootCAs.AppendCertsFromPEM(certs); !ok {
		fatalf("error adding certs to root\n")
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			RootCAs: rootCAs,
		},
	}
}

func fatalf(s string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, s, a...)
	os.Exit(1)
}

func errorf(s string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, s, a...)
	exitCode = 1
}
