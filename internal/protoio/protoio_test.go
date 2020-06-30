package protoio

import (
	"os"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/protoio/testpb"
	"blitiri.com.ar/go/chasquid/internal/testlib"
)

func TestBin(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	pb := &testpb.M{Content: "hola"}

	if err := WriteMessage("f", pb, 0600); err != nil {
		t.Error(err)
	}

	pb2 := &testpb.M{}
	if err := ReadMessage("f", pb2); err != nil {
		t.Error(err)
	}
	if pb.Content != pb2.Content {
		t.Errorf("content mismatch, got %q, expected %q", pb2.Content, pb.Content)
	}
}

func TestText(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	pb := &testpb.M{Content: "hola"}

	if err := WriteTextMessage("f", pb, 0600); err != nil {
		t.Error(err)
	}

	pb2 := &testpb.M{}
	if err := ReadTextMessage("f", pb2); err != nil {
		t.Error(err)
	}
	if pb.Content != pb2.Content {
		t.Errorf("content mismatch, got %q, expected %q", pb2.Content, pb.Content)
	}
}

func TestStore(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	st, err := NewStore(dir + "/store")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if ids, err := st.ListIDs(); len(ids) != 0 || err != nil {
		t.Errorf("expected no ids, got %v - %v", ids, err)
	}

	pb := &testpb.M{Content: "hola"}

	if err := st.Put("f", pb); err != nil {
		t.Error(err)
	}

	pb2 := &testpb.M{}
	if ok, err := st.Get("f", pb2); err != nil || !ok {
		t.Errorf("Get(f): %v - %v", ok, err)
	}
	if pb.Content != pb2.Content {
		t.Errorf("content mismatch, got %q, expected %q", pb2.Content, pb.Content)
	}

	if ok, err := st.Get("notexists", pb2); err != nil || ok {
		t.Errorf("Get(notexists): %v - %v", ok, err)
	}

	// Add an extraneous file, which ListIDs should ignore.
	mustCreate(t, dir+"/store/"+"somefile")

	// Add a file that is not properly query-escaped, and should be ignored.
	mustCreate(t, dir+"/store/"+"s:somefile%N")

	if ids, err := st.ListIDs(); len(ids) != 1 || ids[0] != "f" || err != nil {
		t.Errorf("expected [f], got %v - %v", ids, err)
	}
}

func mustCreate(t *testing.T, fname string) {
	t.Helper()

	f, err := os.Create(fname)
	if f != nil {
		f.Close()
	}
	if err != nil {
		t.Fatalf("failed to create file %q: %v", fname, err)
	}
}

func TestFileErrors(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)
	pb := &testpb.M{Content: "hola"}

	if err := WriteMessage("/proc/doesnotexist", pb, 0600); err == nil {
		t.Errorf("write to /proc/doesnotexist worked, expected error")
	}

	if err := WriteTextMessage("/proc/doesnotexist", pb, 0600); err == nil {
		t.Errorf("text write to /proc/doesnotexist worked, expected error")
	}

	if err := ReadMessage("/doesnotexist", pb); err == nil {
		t.Errorf("read from /doesnotexist worked, expected error")
	}

	if err := ReadTextMessage("/doesnotexist", pb); err == nil {
		t.Errorf("text read from /doesnotexist worked, expected error")
	}

	s := &Store{dir: "/doesnotexist"}
	if ids, err := s.ListIDs(); !(ids == nil && err != nil) {
		t.Errorf("list /doesnotexist worked (%v, %v), expected error", ids, err)
	}
}

func TestMarshalErrors(t *testing.T) {
	dir := testlib.MustTempDir(t)
	defer testlib.RemoveIfOk(t, dir)

	// The marshaller enforces that strings are well-formed utf8. So to create
	// a marshalling error, we use a non-utf8 string.
	pb := &testpb.M{Content: "\xc3\x28"}

	if err := WriteMessage("f", pb, 0600); err == nil {
		t.Errorf("write worked, expected error")
	}

	if err := WriteTextMessage("ft", pb, 0600); err == nil {
		t.Errorf("text write worked, expected error")
	}
}
