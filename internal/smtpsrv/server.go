// Package smtpsrv implements chasquid's SMTP server and connection handler.
package smtpsrv

import (
	"crypto/tls"
	"flag"
	"net"
	"net/http"
	"path"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/userdb"
	"blitiri.com.ar/go/log"
)

var (
	// Reload frequency.
	// We should consider making this a proper option if there's interest in
	// changing it, but until then, it's a test-only flag for simplicity.
	reloadEvery = flag.Duration("testing__reload_every", 30*time.Second,
		"how often to reload, ONLY FOR TESTING")
)

// Server represents an SMTP server instance.
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

	// Use HAProxy on incoming connections.
	HAProxyEnabled bool

	// Local domains.
	localDomains *set.String

	// User databases (per domain).
	// Authenticator.
	authr *auth.Authenticator

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

	// Path to the hooks.
	HookPath string
}

// NewServer returns a new empty Server.
func NewServer() *Server {
	return &Server{
		addrs:          map[SocketMode][]string{},
		listeners:      map[SocketMode][]net.Listener{},
		tlsConfig:      &tls.Config{},
		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		authr:          auth.NewAuthenticator(),
		aliasesR:       aliases.NewResolver(),
	}
}

// AddCerts (TLS) to the server.
func (s *Server) AddCerts(certPath, keyPath string) error {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return err
	}
	s.tlsConfig.Certificates = append(s.tlsConfig.Certificates, cert)
	return nil
}

// AddAddr adds an address for the server to listen on.
func (s *Server) AddAddr(a string, m SocketMode) {
	s.addrs[m] = append(s.addrs[m], a)
}

// AddListeners adds listeners for the server to listen on.
func (s *Server) AddListeners(ls []net.Listener, m SocketMode) {
	s.listeners[m] = append(s.listeners[m], ls...)
}

// AddDomain adds a local domain to the server.
func (s *Server) AddDomain(d string) {
	s.localDomains.Add(d)
	s.aliasesR.AddDomain(d)
}

// AddUserDB adds a userdb.DB instance as backend for the domain.
func (s *Server) AddUserDB(domain string, db *userdb.DB) {
	s.authr.Register(domain, auth.WrapNoErrorBackend(db))
}

// AddAliasesFile adds an aliases file for the given domain.
func (s *Server) AddAliasesFile(domain, f string) error {
	return s.aliasesR.AddAliasesFile(domain, f)
}

// SetAuthFallback sets the authentication backend to use as fallback.
func (s *Server) SetAuthFallback(be auth.Backend) {
	s.authr.Fallback = be
}

// SetAliasesConfig sets the aliases configuration options.
func (s *Server) SetAliasesConfig(suffixSep, dropChars string) {
	s.aliasesR.SuffixSep = suffixSep
	s.aliasesR.DropChars = dropChars
	s.aliasesR.ResolveHook = path.Join(s.HookPath, "alias-resolve")
	s.aliasesR.ExistsHook = path.Join(s.HookPath, "alias-exists")
}

// InitDomainInfo initializes the domain info database.
func (s *Server) InitDomainInfo(dir string) *domaininfo.DB {
	var err error
	s.dinfo, err = domaininfo.New(dir)
	if err != nil {
		log.Fatalf("Error opening domain info database: %v", err)
	}

	return s.dinfo
}

// InitQueue initializes the queue.
func (s *Server) InitQueue(path string, localC, remoteC courier.Courier) {
	q, err := queue.New(path, s.localDomains, s.aliasesR, localC, remoteC)
	if err != nil {
		log.Fatalf("Error initializing queue: %v:", err)
	}

	err = q.Load()
	if err != nil {
		log.Fatalf("Error loading queue: %v", err)
	}
	s.queue = q

	http.HandleFunc("/debug/queue",
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(q.DumpString()))
		})
}

// periodicallyReload some of the server's information, such as aliases and
// the user databases.
func (s *Server) periodicallyReload() {
	if reloadEvery == nil {
		return
	}
	for range time.Tick(*reloadEvery) {
		err := s.aliasesR.Reload()
		if err != nil {
			log.Errorf("Error reloading aliases: %v", err)
		}

		err = s.authr.Reload()
		if err != nil {
			log.Errorf("Error reloading authenticators: %v", err)
		}

		err = s.dinfo.Reload()
		if err != nil {
			log.Errorf("Error reloading domaininfo: %v", err)
		}
	}
}

// ListenAndServe on the addresses and listeners that were previously added.
// This function will not return.
func (s *Server) ListenAndServe() {
	if len(s.tlsConfig.Certificates) == 0 {
		// chasquid assumes there's at least one valid certificate (for things
		// like STARTTLS, user authentication, etc.), so we fail if none was
		// found.
		log.Errorf("No SSL/TLS certificates found")
		log.Errorf("Ideally there should be a certificate for each MX you act as")
		log.Fatalf("At least one valid certificate is needed")
	}

	// At this point the TLS config should be done, build the
	// name->certificate map (used by the TLS library for SNI).
	// TODO: Once we support only Go >= 1.14, we can drop this, as it is no
	// longer necessary.
	s.tlsConfig.BuildNameToCertificate()

	go s.periodicallyReload()

	for m, addrs := range s.addrs {
		for _, addr := range addrs {
			l, err := net.Listen("tcp", addr)
			if err != nil {
				log.Fatalf("Error listening: %v", err)
			}

			log.Infof("Server listening on %s (%v)", addr, m)
			maillog.Listening(addr)
			go s.serve(l, m)
		}
	}

	for m, ls := range s.listeners {
		for _, l := range ls {
			log.Infof("Server listening on %s (%v, via systemd)", l.Addr(), m)
			maillog.Listening(l.Addr().String())
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
	// If this mode is expected to be TLS-wrapped, make it so.
	if mode.TLS {
		l = tls.NewListener(l, s.tlsConfig)
	}

	pdhook := path.Join(s.HookPath, "post-data")

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatalf("Error accepting: %v", err)
		}

		sc := &Conn{
			hostname:       s.Hostname,
			maxDataSize:    s.MaxDataSize,
			postDataHook:   pdhook,
			conn:           conn,
			mode:           mode,
			tlsConfig:      s.tlsConfig,
			haproxyEnabled: s.HAProxyEnabled,
			onTLS:          mode.TLS,
			authr:          s.authr,
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
