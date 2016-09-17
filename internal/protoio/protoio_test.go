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
