// Package domaininfo implements a domain information database, to keep track
// of things we know about a particular domain.
package domaininfo

import (
	"fmt"
	"sync"

	"blitiri.com.ar/go/chasquid/internal/protoio"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

// Command to generate domaininfo.pb.go.
//go:generate protoc --go_out=. domaininfo.proto

// DB represents the persistent domain information database.
type DB struct {
	// Persistent store with the list of domains we know.
	store *protoio.Store

	info map[string]*Domain
	sync.Mutex

	ev *trace.EventLog
}

// New opens a domain information database on the given dir, creating it if
// necessary. The returned database will not be loaded.
func New(dir string) (*DB, error) {
	st, err := protoio.NewStore(dir)
	if err != nil {
		return nil, err
	}

	l := &DB{
		store: st,
		info:  map[string]*Domain{},
	}
	l.ev = trace.NewEventLog("DomainInfo", dir)

	err = l.Reload()
	if err != nil {
		return nil, err
	}

	return l, nil
}

// Reload the database from disk.
func (db *DB) Reload() error {
	db.Lock()
	defer db.Unlock()

	// Clear the map, in case it has data.
	db.info = map[string]*Domain{}

	ids, err := db.store.ListIDs()
	if err != nil {
		return err
	}

	for _, id := range ids {
		d := &Domain{}
		_, err := db.store.Get(id, d)
		if err != nil {
			return fmt.Errorf("error loading %q: %v", id, err)
		}

		db.info[d.Name] = d
	}

	db.ev.Debugf("loaded %d domains", len(ids))
	return nil
}

func (db *DB) write(d *Domain) {
	err := db.store.Put(d.Name, d)
	if err != nil {
		db.ev.Errorf("%s error saving: %v", d.Name, err)
	} else {
		db.ev.Debugf("%s saved", d.Name)
	}
}

// IncomingSecLevel checks an incoming security level for the domain.
// Returns true if allowed, false otherwise.
func (db *DB) IncomingSecLevel(domain string, level SecLevel) bool {
	db.Lock()
	defer db.Unlock()

	d, exists := db.info[domain]
	if !exists {
		d = &Domain{Name: domain}
		db.info[domain] = d
		defer db.write(d)
	}

	if level < d.IncomingSecLevel {
		db.ev.Errorf("%s incoming denied: %s < %s",
			d.Name, level, d.IncomingSecLevel)
		return false
	} else if level == d.IncomingSecLevel {
		db.ev.Debugf("%s incoming allowed: %s == %s",
			d.Name, level, d.IncomingSecLevel)
		return true
	} else {
		db.ev.Printf("%s incoming level raised: %s > %s",
			d.Name, level, d.IncomingSecLevel)
		d.IncomingSecLevel = level
		if exists {
			defer db.write(d)
		}
		return true
	}
}

// OutgoingSecLevel checks an incoming security level for the domain.
// Returns true if allowed, false otherwise.
func (db *DB) OutgoingSecLevel(domain string, level SecLevel) bool {
	db.Lock()
	defer db.Unlock()

	d, exists := db.info[domain]
	if !exists {
		d = &Domain{Name: domain}
		db.info[domain] = d
		defer db.write(d)
	}

	if level < d.OutgoingSecLevel {
		db.ev.Errorf("%s outgoing denied: %s < %s",
			d.Name, level, d.OutgoingSecLevel)
		return false
	} else if level == d.OutgoingSecLevel {
		db.ev.Debugf("%s outgoing allowed: %s == %s",
			d.Name, level, d.OutgoingSecLevel)
		return true
	} else {
		db.ev.Printf("%s outgoing level raised: %s > %s",
			d.Name, level, d.OutgoingSecLevel)
		d.OutgoingSecLevel = level
		if exists {
			defer db.write(d)
		}
		return true
	}
}
