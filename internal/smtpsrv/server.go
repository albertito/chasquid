// Package smtpsrv implements chasquid's SMTP server and connection handler.
package smtpsrv

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/textproto"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"github.com/golang/glog"
)

type Server struct {
	// Main hostname, used for display only.
	Hostname string

	// Maximum data size.
	MaxDataSize int64

	// Addresses.
	addrs map[SocketMode][]string

	// Listeners (that came via systemd).
	listeners map[SocketMode][]net.Listener

	// TLS config (including loaded certificates).
	tlsConfig *tls.Config

	// Local domains.
	localDomains *set.String

	// User databases (per domain).
	userDBs map[string]*userdb.DB

	// Aliases resolver.
	aliasesR *aliases.Resolver

	// Domain info database.
	dinfo *domaininfo.DB

	// Time before we give up on a connection, even if it's sending data.
	connTimeout time.Duration

	// Time we wait for command round-trips (excluding DATA).
	commandTimeout time.Duration

	// Queue where we put incoming mail.
	queue *queue.Queue

	// Path to the Post-DATA hook.
	PostDataHook string
}

func NewServer() *Server {
	return &Server{
		addrs:          map[SocketMode][]string{},
		listeners:      map[SocketMode][]net.Listener{},
		tlsConfig:      &tls.Config{},
		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		userDBs:        map[string]*userdb.DB{},
		aliasesR:       aliases.NewResolver(),
	}
}

func (s *Server) AddCerts(certPath, keyPath string) error {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return err
	}
	s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, cert)
	return nil
}

func (s *Server) AddAddr(a string, m SocketMode) {
	s.addrs[m] = append(s.addrs[m], a)
}

func (s *Server) AddListeners(ls []net.Listener, m SocketMode) {
	s.listeners[m] = append(s.listeners[m], ls...)
}

func (s *Server) AddDomain(d string) {
	s.localDomains.Add(d)
	s.aliasesR.AddDomain(d)
}

func (s *Server) AddUserDB(domain string, db *userdb.DB) {
	s.userDBs[domain] = db
}

func (s *Server) AddAliasesFile(domain, f string) error {
	return s.aliasesR.AddAliasesFile(domain, f)
}

func (s *Server) SetAliasesConfig(suffixSep, dropChars string) {
	s.aliasesR.SuffixSep = suffixSep
	s.aliasesR.DropChars = dropChars
}

func (s *Server) InitDomainInfo(dir string) *domaininfo.DB {
	var err error
	s.dinfo, err = domaininfo.New(dir)
	if err != nil {
		glog.Fatalf("Error opening domain info database: %v", err)
	}

	err = s.dinfo.Load()
	if err != nil {
		glog.Fatalf("Error loading domain info database: %v", err)
	}

	return s.dinfo
}

func (s *Server) InitQueue(path string, localC, remoteC courier.Courier) {
	q := queue.New(path, s.localDomains, s.aliasesR, localC, remoteC, s.Hostname)
	err := q.Load()
	if err != nil {
		glog.Fatalf("Error loading queue: %v", err)
	}
	s.queue = q

	http.HandleFunc("/debug/queue",
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(q.DumpString()))
		})
}

// PeriodicallyReload some of the server's information, such as aliases and
// the user databases.
func (s *Server) periodicallyReload() {
	for range time.Tick(30 * time.Second) {
		err := s.aliasesR.Reload()
		if err != nil {
			glog.Errorf("Error reloading aliases: %v", err)
		}

		for domain, udb := range s.userDBs {
			err = udb.Reload()
			if err != nil {
				glog.Errorf("Error reloading %q user db: %v", domain, err)
			}
		}
	}
}

func (s *Server) ListenAndServe() {
	// At this point the TLS config should be done, build the
	// name->certificate map (used by the TLS library for SNI).
	s.tlsConfig.BuildNameToCertificate()

	go s.periodicallyReload()

	for m, addrs := range s.addrs {
		for _, addr := range addrs {
			l, err := net.Listen("tcp", addr)
			if err != nil {
				glog.Fatalf("Error listening: %v", err)
			}

			glog.Infof("Server listening on %s (%v)", addr, m)
			go s.serve(l, m)
		}
	}

	for m, ls := range s.listeners {
		for _, l := range ls {
			glog.Infof("Server listening on %s (%v, via systemd)", l.Addr(), m)
			go s.serve(l, m)
		}
	}

	// Never return. If the serve goroutines have problems, they will abort
	// execution.
	for {
		time.Sleep(24 * time.Hour)
	}
}

func (s *Server) serve(l net.Listener, mode SocketMode) {
	for {
		conn, err := l.Accept()
		if err != nil {
			glog.Fatalf("Error accepting: %v", err)
		}

		sc := &Conn{
			hostname:       s.Hostname,
			maxDataSize:    s.MaxDataSize,
			postDataHook:   s.PostDataHook,
			conn:           conn,
			tc:             textproto.NewConn(conn),
			mode:           mode,
			tlsConfig:      s.tlsConfig,
			userDBs:        s.userDBs,
			aliasesR:       s.aliasesR,
			localDomains:   s.localDomains,
			dinfo:          s.dinfo,
			deadline:       time.Now().Add(s.connTimeout),
			commandTimeout: s.commandTimeout,
			queue:          s.queue,
		}
		go sc.Handle()
	}
}
