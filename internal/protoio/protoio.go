// Package protoio contains I/O functions for protocol buffers.
package protoio

import (
	"net/url"
	"os"
	"strings"

	"blitiri.com.ar/go/chasquid/internal/safeio"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

// ReadMessage reads a protocol buffer message from fname, and unmarshalls it
// into pb.
func ReadMessage(fname string, pb proto.Message) error {
	in, err := os.ReadFile(fname)
	if err != nil {
		return err
	}
	return proto.Unmarshal(in, pb)
}

// ReadTextMessage reads a text format protocol buffer message from fname, and
// unmarshalls it into pb.
func ReadTextMessage(fname string, pb proto.Message) error {
	in, err := os.ReadFile(fname)
	if err != nil {
		return err
	}
	return prototext.Unmarshal(in, pb)
}

// WriteMessage marshals pb and atomically writes it into fname.
func WriteMessage(fname string, pb proto.Message, perm os.FileMode) error {
	out, err := proto.Marshal(pb)
	if err != nil {
		return err
	}

	return safeio.WriteFile(fname, out, perm)
}

var textOpts = prototext.MarshalOptions{
	Multiline: true,
}

// WriteTextMessage marshals pb in text format and atomically writes it into
// fname.
func WriteTextMessage(fname string, pb proto.Message, perm os.FileMode) error {
	out, err := textOpts.Marshal(pb)
	if err != nil {
		return err
	}
	return safeio.WriteFile(fname, out, perm)
}

///////////////////////////////////////////////////////////////

// Store represents a persistent protocol buffer message store.
type Store struct {
	// Directory where the store is.
	dir string
}

// NewStore returns a new Store instance.  It will create dir if needed.
func NewStore(dir string) (*Store, error) {
	s := &Store{dir}
	err := os.MkdirAll(dir, 0770)
	return s, err
}

const storeIDPrefix = "s:"

// idToFname takes a generic id and returns the corresponding file for it
// (which may or may not exist).
func (s *Store) idToFname(id string) string {
	return s.dir + "/" + storeIDPrefix + url.QueryEscape(id)
}

// Put a message into the store.
func (s *Store) Put(id string, m proto.Message) error {
	return WriteTextMessage(s.idToFname(id), m, 0660)
}

// Get a message from the store.
func (s *Store) Get(id string, m proto.Message) (bool, error) {
	err := ReadTextMessage(s.idToFname(id), m)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// ListIDs in the store.
func (s *Store) ListIDs() ([]string, error) {
	ids := []string{}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), storeIDPrefix) {
			continue
		}

		id := e.Name()[len(storeIDPrefix):]
		id, err = url.QueryUnescape(id)
		if err != nil {
			continue
		}

		ids = append(ids, id)
	}

	return ids, nil
}
