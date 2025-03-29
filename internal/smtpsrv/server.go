// Package smtpsrv implements chasquid's SMTP server and connection handler.
package smtpsrv

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/aliases"
	"blitiri.com.ar/go/chasquid/internal/auth"
	"blitiri.com.ar/go/chasquid/internal/courier"
	"blitiri.com.ar/go/chasquid/internal/dkim"
	"blitiri.com.ar/go/chasquid/internal/domaininfo"
	"blitiri.com.ar/go/chasquid/internal/localrpc"
	"blitiri.com.ar/go/chasquid/internal/maillog"
	"blitiri.com.ar/go/chasquid/internal/queue"
	"blitiri.com.ar/go/chasquid/internal/set"
	"blitiri.com.ar/go/chasquid/internal/trace"
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

	// Map of domain -> DKIM signers.
	dkimSigners map[string][]*dkim.Signer

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
	authr := auth.NewAuthenticator()
	aliasesR := aliases.NewResolver(authr.Exists)
	return &Server{
		addrs:     map[SocketMode][]string{},
		listeners: map[SocketMode][]net.Listener{},

		// Disable session tickets for now, to workaround a Microsoft bug
		// causing deliverability issues.
		//
		// See https://github.com/golang/go/issues/70232 for more details.
		//
		// This doesn't impact security, it just makes the re-establishment of
		// TLS sessions a bit slower, but for a server like chasquid it's not
		// going to be significant.
		//
		// Note this is not a Go-specific problem, and affects other servers
		// too (like Postfix/OpenSSL). This is a Microsoft problem that they
		// need to fix. Unfortunately, because they're quite a big provider
		// and are not very responsive in fixing their problems, we have to do
		// a workaround here.
		// TODO: Remove this once Microsoft fixes their servers.
		tlsConfig: &tls.Config{
			SessionTicketsDisabled: true,
		},

		connTimeout:    20 * time.Minute,
		commandTimeout: 1 * time.Minute,
		localDomains:   &set.String{},
		authr:          authr,
		aliasesR:       aliasesR,
		dkimSigners:    map[string][]*dkim.Signer{},
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

// AddUserDB adds a userdb file as backend for the domain.
func (s *Server) AddUserDB(domain, f string) (int, error) {
	// Load the userdb, and register it unconditionally (so reload works even
	// if there are errors right now).
	udb, err := userdb.Load(f)
	s.authr.Register(domain, auth.WrapNoErrorBackend(udb))
	return udb.Len(), err
}

// AddAliasesFile adds an aliases file for the given domain.
func (s *Server) AddAliasesFile(domain, f string) (int, error) {
	return s.aliasesR.AddAliasesFile(domain, f)
}

var (
	errDecodingPEMBlock     = fmt.Errorf("error decoding PEM block")
	errUnsupportedBlockType = fmt.Errorf("unsupported block type")
	errUnsupportedKeyType   = fmt.Errorf("unsupported key type")
)

// AddDKIMSigner for the given domain and selector.
func (s *Server) AddDKIMSigner(domain, selector, keyPath string) error {
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(key)
	if block == nil {
		return errDecodingPEMBlock
	}

	if strings.ToUpper(block.Type) != "PRIVATE KEY" {
		return fmt.Errorf("%w: %s", errUnsupportedBlockType, block.Type)
	}

	signer, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}

	switch k := signer.(type) {
	case *rsa.PrivateKey, ed25519.PrivateKey:
		// These are supported, nothing to do.
	default:
		return fmt.Errorf("%w: %T", errUnsupportedKeyType, k)
	}

	s.dkimSigners[domain] = append(s.dkimSigners[domain], &dkim.Signer{
		Domain:   domain,
		Selector: selector,
		Signer:   signer.(crypto.Signer),
	})
	return nil
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
}

// SetDomainInfo sets the domain info database to use.
func (s *Server) SetDomainInfo(dinfo *domaininfo.DB) {
	s.dinfo = dinfo
}

// InitQueue initializes the queue.
func (s *Server) InitQueue(path string, localC, remoteC courier.Courier) {
	q, err := queue.New(path, s.localDomains, s.aliasesR, localC, remoteC)
	if err != nil {
		log.Fatalf("Error initializing queue: %v", err)
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

func (s *Server) aliasResolveRPC(tr *trace.Trace, req url.Values) (url.Values, error) {
	rcpts, err := s.aliasesR.Resolve(tr, req.Get("Address"))
	if err != nil {
		return nil, err
	}

	v := url.Values{}
	for _, rcpt := range rcpts {
		v.Add(string(rcpt.Type), rcpt.Addr)
	}

	return v, nil
}

func (s *Server) dinfoClearRPC(tr *trace.Trace, req url.Values) (url.Values, error) {
	domain := req.Get("Domain")
	exists := s.dinfo.Clear(tr, domain)
	if !exists {
		return nil, fmt.Errorf("does not exist")
	}
	return nil, nil
}

// periodicallyReload some of the server's information that can be changed
// without the server knowing, such as aliases and the user databases.
func (s *Server) periodicallyReload() {
	if reloadEvery == nil {
		return
	}

	//lint:ignore SA1015 This lasts the program's lifetime.
	for range time.Tick(*reloadEvery) {
		s.Reload()
	}
}

func (s *Server) Reload() {
	// Note that any error while reloading is fatal: this way, if there is an
	// unexpected error it can be detected (and corrected) quickly, instead of
	// much later (e.g. upon restart) when it might be harder to debug.
	if err := s.aliasesR.Reload(); err != nil {
		log.Fatalf("Error reloading aliases: %v", err)
	}

	if err := s.authr.Reload(); err != nil {
		log.Fatalf("Error reloading authenticators: %v", err)
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

	localrpc.DefaultServer.Register("AliasResolve", s.aliasResolveRPC)
	localrpc.DefaultServer.Register("DomaininfoClear", s.dinfoClearRPC)

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
			dkimSigners:    s.dkimSigners,
			deadline:       time.Now().Add(s.connTimeout),
			commandTimeout: s.commandTimeout,
			queue:          s.queue,
		}
		go sc.Handle()
	}
}
