// Package protoio contains I/O functions for protocol buffers.
package protoio

import (
	"io/ioutil"
	"os"

	"blitiri.com.ar/go/chasquid/internal/safeio"

	"github.com/golang/protobuf/proto"
)

// ReadMessage reads a protocol buffer message from fname, and unmarshalls it
// into pb.
func ReadMessage(fname string, pb proto.Message) error {
	in, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}
	return proto.Unmarshal(in, pb)
}

// ReadTextMessage reads a text format protocol buffer message from fname, and
// unmarshalls it into pb.
func ReadTextMessage(fname string, pb proto.Message) error {
	in, err := ioutil.ReadFile(fname)
	if err != nil {
		return err
	}
	return proto.UnmarshalText(string(in), pb)
}

// WriteMessage marshals pb and atomically writes it into fname.
func WriteMessage(fname string, pb proto.Message, perm os.FileMode) error {
	out, err := proto.Marshal(pb)
	if err != nil {
		return err
	}

	return safeio.WriteFile(fname, out, perm)
}

// WriteTextMessage marshals pb in text format and atomically writes it into
// fname.
func WriteTextMessage(fname string, pb proto.Message, perm os.FileMode) error {
	out := proto.MarshalTextString(pb)
	return safeio.WriteFile(fname, []byte(out), perm)
}
