package localrpc

import (
	"errors"
	"net"
	"net/url"
	"os"
	"testing"
	"time"

	"blitiri.com.ar/go/chasquid/internal/trace"
	"github.com/google/go-cmp/cmp"
)

func Echo(tr *trace.Trace, input url.Values) (url.Values, error) {
	return input, nil
}

func Hola(tr *trace.Trace, input url.Values) (url.Values, error) {
	output := url.Values{}
	output.Set("greeting", "Hola "+input.Get("name"))
	return output, nil
}

var testErr = errors.New("test error")

func HolaErr(tr *trace.Trace, input url.Values) (url.Values, error) {
	return nil, testErr
}

type testServer struct {
	dir  string
	sock string
	*Server
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "rpc-test-*")
	if err != nil {
		t.Fatal(err)
	}

	tsrv := &testServer{
		dir:    tmpDir,
		sock:   tmpDir + "/sock",
		Server: NewServer(),
	}

	tsrv.Register("Echo", Echo)
	tsrv.Register("Hola", Hola)
	tsrv.Register("HolaErr", HolaErr)
	go tsrv.ListenAndServe(tsrv.sock)

	waitForServer(t, tsrv.sock)
	return tsrv
}

func (tsrv *testServer) Cleanup() {
	tsrv.Close()
	os.RemoveAll(tsrv.dir)
}

func mkV(args ...string) url.Values {
	v := url.Values{}
	for i := 0; i < len(args); i += 2 {
		v.Set(args[i], args[i+1])
	}
	return v
}

func TestEndToEnd(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Cleanup()

	// Run the client.
	client := NewClient(srv.sock)

	cases := []struct {
		method string
		input  url.Values
		output url.Values
		err    error
	}{
		{"Echo", nil, mkV(), nil},
		{"Echo", mkV("msg", "hola"), mkV("msg", "hola"), nil},
		{"Hola", mkV("name", "marola"), mkV("greeting", "Hola marola"), nil},
		{"HolaErr", nil, nil, testErr},
		{"UnknownMethod", nil, nil, errUnknownMethod},
	}

	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			resp, err := client.CallWithValues(c.method, c.input)
			if diff := cmp.Diff(c.err, err, transformErrors); diff != "" {
				t.Errorf("error mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(c.output, resp); diff != "" {
				t.Errorf("output mismatch (-want +got):\n%s", diff)
			}
		})
	}

	// Check Call too.
	output, err := client.Call("Hola", "name", "marola")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if diff := cmp.Diff(mkV("greeting", "Hola marola"), output); diff != "" {
		t.Errorf("output mismatch (-want +got):\n%s", diff)
	}
}

func waitForServer(t *testing.T, path string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		time.Sleep(10 * time.Millisecond)
		conn, err := net.Dial("unix", path)
		if conn != nil {
			conn.Close()
		}
		if err == nil {
			return
		}
	}
	t.Fatal("server didn't start")
}

// Allow us to compare errors with cmp.Diff by their string content (since the
// instances/types don't carry across RPC boundaries).
var transformErrors = cmp.Transformer(
	"error",
	func(err error) string {
		if err == nil {
			return "<nil>"
		}
		return err.Error()
	})
