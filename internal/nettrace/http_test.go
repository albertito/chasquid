package nettrace

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func getValues(t *testing.T, vs url.Values, code int) string {
	t.Helper()

	req := httptest.NewRequest("GET", "/debug/traces?"+vs.Encode(), nil)
	w := httptest.NewRecorder()
	RenderTraces(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != code {
		t.Errorf("expected %d, got %v", code, resp)
	}

	return string(body)
}

type v struct {
	fam, b, lat, trace, ref, all string
}

func getCode(t *testing.T, vs v, code int) string {
	t.Helper()

	u := url.Values{}
	if vs.fam != "" {
		u.Add("fam", vs.fam)
	}
	if vs.b != "" {
		u.Add("b", vs.b)
	}
	if vs.lat != "" {
		u.Add("lat", vs.lat)
	}
	if vs.trace != "" {
		u.Add("trace", vs.trace)
	}
	if vs.ref != "" {
		u.Add("ref", vs.ref)
	}
	if vs.all != "" {
		u.Add("all", vs.all)
	}

	return getValues(t, u, code)
}

func get(t *testing.T, fam, b, lat, trace, ref, all string) string {
	t.Helper()
	return getCode(t, v{fam, b, lat, trace, ref, all}, 200)
}

func getErr(t *testing.T, fam, b, lat, trace, ref, all string, code int, err string) string {
	t.Helper()
	body := getCode(t, v{fam, b, lat, trace, ref, all}, code)
	if !strings.Contains(body, err) {
		t.Errorf("Body does not contain error message %q", err)
		t.Logf("Body: %v", body)
	}

	return body
}

func checkContains(t *testing.T, body, s string) {
	t.Helper()
	if !strings.Contains(body, s) {
		t.Errorf("Body does not contain %q", s)
		t.Logf("Body: %v", body)
	}
}

func TestHTTP(t *testing.T) {
	tr := New("TestHTTP", "http")
	tr.Printf("entry #1")
	tr.Finish()

	tr = New("TestHTTP", "http")
	tr.Printf("entry #2")
	tr.Finish()

	tr = New("TestHTTP", "http")
	tr.Errorf("entry #3 (error)")
	tr.Finish()

	tr = New("TestHTTP", "http")
	tr.Printf("hola marola")
	tr.Printf("entry #4")
	// This one is active until the end.
	defer tr.Finish()

	// Get the plain index.
	body := get(t, "", "", "", "", "", "")
	checkContains(t, body, "TestHTTP")

	// Get a specific family, but no bucket.
	body = get(t, "TestHTTP", "", "", "", "", "")
	checkContains(t, body, "TestHTTP")

	// Get a family and active bucket.
	body = get(t, "TestHTTP", "-1", "", "", "", "")
	checkContains(t, body, "hola marola")

	// Get a family and error bucket.
	body = get(t, "TestHTTP", "-2", "", "", "", "")
	checkContains(t, body, "entry #3 (error)")

	// Get a family and first bucket.
	body = get(t, "TestHTTP", "0", "", "", "", "")
	checkContains(t, body, "entry #2")

	// Latency view. There are 3 events because the 4th is active.
	body = get(t, "TestHTTP", "", "lat", "", "", "")
	checkContains(t, body, "Count: 3")

	// Get a specific trace. No family given, since it shouldn't be needed (we
	// take it from the id).
	body = get(t, "", "", "", string(tr.(*trace).ID), "", "")
	checkContains(t, body, "hola marola")

	// Check the "all=true" views.
	body = get(t, "TestHTTP", "0", "", "", "", "true")
	checkContains(t, body, "entry #2")
	checkContains(t, body, "?fam=TestHTTP&b=-2&all=true")

	tr.Finish()
}

func TestHTTPLong(t *testing.T) {
	// Test a long trace.
	tr := New("TestHTTPLong", "verbose")
	for i := 0; i < 1000; i++ {
		tr.Printf("entry #%d", i)
	}
	tr.Finish()
	get(t, "TestHTTPLong", "", "", string(tr.(*trace).ID), "", "")
}

func TestHTTPErrors(t *testing.T) {
	tr := New("TestHTTPErrors", "http")
	tr.Printf("entry #1")
	tr.Finish()

	// Unknown family.
	getErr(t, "unkfamily", "", "", "", "", "",
		404, "Unknown family")

	// Invalid bucket.
	getErr(t, "TestHTTPErrors", "abc", "", "", "", "",
		400, "Invalid bucket")
	getErr(t, "TestHTTPErrors", "-3", "", "", "", "",
		400, "Invalid bucket")
	getErr(t, "TestHTTPErrors", "9", "", "", "", "",
		400, "Invalid bucket")

	// Unknown trace id (malformed).
	getErr(t, "TestHTTPErrors", "", "", "unktrace", "", "",
		404, "Trace not found")

	// Unknown trace id.
	getErr(t, "TestHTTPErrors", "", "", string(tr.(*trace).ID)+"xxx", "", "",
		404, "Trace not found")

	// Check that the trace is actually there.
	get(t, "", "", "", string(tr.(*trace).ID), "", "")
}

func TestHTTPUroboro(t *testing.T) {
	trA := New("TestHTTPUroboro", "trA")
	defer trA.Finish()
	trA.Printf("this is trace A")

	trB := New("TestHTTPUroboro", "trB")
	defer trB.Finish()
	trB.Printf("this is trace B")

	trA.Link(trB, "B is my friend")
	trB.Link(trA, "A is my friend")

	// Check that we handle cross-linked events well.
	get(t, "TestHTTPUroboro", "", "", "", "", "")
	get(t, "TestHTTPUroboro", "-1", "", "", "", "")
	get(t, "", "", "", string(trA.(*trace).ID), "", "")
	get(t, "", "", "", string(trB.(*trace).ID), "", "")
}

func TestHTTPDeep(t *testing.T) {
	tr := New("TestHTTPDeep", "level-0")
	defer tr.Finish()
	ts := []Trace{tr}
	for i := 1; i <= 9; i++ {
		tr = tr.NewChild("TestHTTPDeep", fmt.Sprintf("level-%d", i))
		defer tr.Finish()
		ts = append(ts, tr)
	}

	// Active view.
	body := get(t, "TestHTTPDeep", "-1", "", "", "", "")
	checkContains(t, body, "level-9")

	// Recursive view.
	body = get(t, "TestHTTPDeep", "", "", string(ts[0].(*trace).ID), "", "")
	checkContains(t, body, "level-9")
}

func TestStripZeros(t *testing.T) {
	cases := []struct {
		d   time.Duration
		exp string
	}{
		{0 * time.Second, " .     0"},
		{1 * time.Millisecond, " .  1000"},
		{5 * time.Millisecond, " .  5000"},
		{1 * time.Second, "1.000000"},
		{1*time.Second + 8*time.Millisecond, "1.008000"},
	}
	for _, c := range cases {
		if got := stripZeros(c.d); got != c.exp {
			t.Errorf("stripZeros(%s) got %q, expected %q",
				c.d, got, c.exp)
		}
	}
}

func TestRegisterHandler(t *testing.T) {
	mux := http.NewServeMux()
	RegisterHandler(mux)

	req := httptest.NewRequest("GET", "/debug/traces", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %v", resp)
	}

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<h1>Traces</h1>") {
		t.Errorf("unexpected body: %s", body)
	}
}
