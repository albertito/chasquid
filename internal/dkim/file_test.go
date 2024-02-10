package dkim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFromFiles(t *testing.T) {
	msgfs, err := filepath.Glob("testdata/*.msg")
	if err != nil {
		t.Fatalf("error finding test files: %v", err)
	}

	for _, msgf := range msgfs {
		base := strings.TrimSuffix(msgf, filepath.Ext(msgf))
		t.Run(base, func(t *testing.T) { testOne(t, base) })
	}
}

// This is the same as TestFromFiles, but it runs the private test files,
// which are not included in the git repository.
// This is useful for running tests on your own machine, with emails that you
// don't necessarily want to share publicly.
func TestFromPrivateFiles(t *testing.T) {
	msgfs, err := filepath.Glob("testdata/private/*/*.msg")
	if err != nil {
		t.Fatalf("error finding private test files: %v", err)
	}

	for _, msgf := range msgfs {
		base := strings.TrimSuffix(msgf, filepath.Ext(msgf))
		t.Run(base, func(t *testing.T) { testOne(t, base) })
	}
}

func testOne(t *testing.T, base string) {
	ctx := context.Background()
	ctx = WithTraceFunc(ctx, t.Logf)

	ctx = loadDNS(t, ctx, base+".dns")
	msg := toCRLF(mustReadFile(t, base+".msg"))
	wantResult := loadResult(t, base+".result")
	wantError := loadError(t, base+".error")

	t.Logf("Message: %.60q", msg)
	t.Logf("Want result: %+v", wantResult)
	t.Logf("Want error: %v", wantError)

	res, err := VerifyMessage(ctx, msg)

	// Write the results out for easy updating.
	writeResults(t, base, res, err)

	diff := cmp.Diff(wantResult, res, cmp.Comparer(equalErrors))
	if diff != "" {
		t.Errorf("VerifyMessage result diff (-want +got):\n%s", diff)
	}

	// We need to compare them by hand because cmp.Diff won't use our comparer
	// for top-level errors.
	if !equalErrors(wantError, err) {
		diff := cmp.Diff(wantError, err)
		t.Errorf("VerifyMessage error diff (-want +got):\n%s", diff)
	}
}

// Used to make cmp.Diff compare errors by their messages. This is obviously
// not great, but it's good enough for this test.
func equalErrors(a, b error) bool {
	if a == nil {
		return b == nil
	}
	if b == nil {
		return false
	}
	return a.Error() == b.Error()
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatalf("error reading %q: %v", path, err)
	}
	return string(contents)
}

func loadDNS(t *testing.T, ctx context.Context, path string) context.Context {
	t.Helper()

	results := map[string][]string{}
	errors := map[string]error{}
	txtFunc := func(ctx context.Context, domain string) ([]string, error) {
		return results[domain], errors[domain]
	}
	ctx = WithLookupTXTFunc(ctx, txtFunc)

	c := mustReadFile(t, path)

	// Unfold \-terminated lines.
	c = strings.ReplaceAll(c, "\\\n", "")

	for _, line := range strings.Split(c, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		domain, txt, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		domain = strings.TrimSpace(domain)

		switch strings.TrimSpace(txt) {
		case "TEMPERROR":
			errors[domain] = &net.DNSError{
				Err:         "temporary error (for testing)",
				IsTemporary: true,
			}
		case "PERMERROR":
			errors[domain] = &net.DNSError{
				Err:         "permanent error (for testing)",
				IsTemporary: false,
			}
		case "NOTFOUND":
			errors[domain] = &net.DNSError{
				Err:        "domain not found (for testing)",
				IsNotFound: true,
			}
		default:
			results[domain] = append(results[domain], txt)
		}
	}

	t.Logf("Loaded DNS results: %#v", results)
	t.Logf("Loaded DNS errors: %v", errors)
	return ctx
}

func loadResult(t *testing.T, path string) *VerifyResult {
	t.Helper()

	res := &VerifyResult{}
	c := mustReadFile(t, path)
	if c == "" {
		return nil
	}

	err := json.Unmarshal([]byte(c), res)
	if err != nil {
		t.Fatalf("error unmarshalling %q: %v", path, err)
	}
	return res
}

func loadError(t *testing.T, path string) error {
	t.Helper()

	c := strings.TrimSpace(mustReadFile(t, path))
	if c == "" || c == "nil" || c == "<nil>" {
		return nil
	}
	return errors.New(c)
}

func mustWriteFile(t *testing.T, path string, c []byte) {
	t.Helper()
	err := os.WriteFile(path, c, 0644)
	if err != nil {
		t.Fatalf("error writing %q: %v", path, err)
	}
}

func writeResults(t *testing.T, base string, res *VerifyResult, err error) {
	t.Helper()

	mustWriteFile(t, base+".error.got", []byte(fmt.Sprintf("%v", err)))

	c, err := json.MarshalIndent(res, "", "\t")
	if err != nil {
		t.Fatalf("error marshalling result: %v", err)
	}
	mustWriteFile(t, base+".result.got", c)
}

// Custom json marshaller so we can write errors as strings.
func (or *OneResult) MarshalJSON() ([]byte, error) {
	// We use an alias to avoid infinite recursion.
	type Alias OneResult
	aux := &struct {
		Error string `json:""`
		*Alias
	}{
		Alias: (*Alias)(or),
	}
	if or.Error != nil {
		aux.Error = or.Error.Error()
	}

	return json.Marshal(aux)
}

// Custom json unmarshaller so we can read errors as strings.
func (or *OneResult) UnmarshalJSON(b []byte) error {
	// We use an alias to avoid infinite recursion.
	type Alias OneResult
	aux := &struct {
		Error string `json:""`
		*Alias
	}{
		Alias: (*Alias)(or),
	}
	if err := json.Unmarshal(b, aux); err != nil {
		return err
	}

	if aux.Error != "" {
		or.Error = errors.New(aux.Error)
	}
	return nil
}
