package protoio

import (
	"io/ioutil"
	"os"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/protoio/testpb"
)

func mustTempDir(t *testing.T) string {
	dir, err := ioutil.TempDir("", "safeio_test")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Chdir(dir)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("test directory: %q", dir)

	return dir
}

func TestBin(t *testing.T) {
	dir := mustTempDir(t)
	pb := &testpb.M{"hola"}

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

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestText(t *testing.T) {
	dir := mustTempDir(t)
	pb := &testpb.M{"hola"}

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

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}

func TestStore(t *testing.T) {
	dir := mustTempDir(t)
	st, err := NewStore(dir + "/store")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if ids, err := st.ListIDs(); len(ids) != 0 || err != nil {
		t.Errorf("expected no ids, got %v - %v", ids, err)
	}

	pb := &testpb.M{"hola"}

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

	if ids, err := st.ListIDs(); len(ids) != 1 || ids[0] != "f" || err != nil {
		t.Errorf("expected [f], got %v - %v", ids, err)
	}

	if !t.Failed() {
		os.RemoveAll(dir)
	}
}
