// Package aliases implements an email aliases resolver.
//
// The resolver can parse many files for different domains, and perform
// lookups to resolve the aliases.
//
//
// File format
//
// It generally follows the traditional aliases format used by sendmail and
// exim.
//
// The file can contain lines of the form:
//
//   user: address, address
//   user: | command
//
// Lines starting with "#" are ignored, as well as empty lines.
// User names cannot contain spaces, ":" or commas, for parsing reasons. This
// is a tradeoff between flexibility and keeping the file format easy to edit
// for people.
//
// User names will be normalized internally to lower-case.
//
// Usually there will be one database per domain, and there's no need to
// include the "@" in the user (in this case, "@" will be forbidden).
//
//
// Recipients
//
// Recipients can be of different types:
//  - Email: the usual user@domain we all know and love, this is the default.
//  - Pipe: if the right side starts with "| ", the rest of the line specifies
//      a command to pipe the email through.
//      Command and arguments are space separated. No quoting, escaping, or
//      replacements of any kind.
//
//
// Lookups
//
// The resolver will perform lookups recursively, until it finds all the final
// recipients.
//
// There are recursion limits to avoid alias loops. If the limit is reached,
// the entire resolution will fail.
//
//
// Suffix removal
//
// The resolver can also remove suffixes from emails, and drop characters
// completely. This can be used to turn "user+blah@domain" into "user@domain",
// and "us.er@domain" into "user@domain".
//
// Both are optional, and the characters configurable globally.
package aliases

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/expvarom"
	"blitiri.com.ar/go/chasquid/internal/normalize"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

// Exported variables.
var (
	hookResults = expvarom.NewMap("chasquid/aliases/hookResults",
		"result", "count of aliases hook results, by hook and result")
)

// Recipient represents a single recipient, after resolving aliases.
// They don't have any special interface, the callers will do a type switch
// anyway.
type Recipient struct {
	Addr string
	Type RType
}

// RType represents a recipient type, see the contants below for valid values.
type RType string

// Valid recipient types.
const (
	EMAIL RType = "(email)"
	PIPE  RType = "(pipe)"
)

// Special token used to define the catch-all addresses.
const (
	CATCH_ALL_TOKEN = "_"
)

var (
	// ErrRecursionLimitExceeded is returned when the resolving lookup
	// exceeded the recursion limit. Usually caused by aliases loops.
	ErrRecursionLimitExceeded = fmt.Errorf("recursion limit exceeded")

	// How many levels of recursions we allow during lookups.
	// We don't expect much recursion, so keeping this low to catch errors
	// quickly.
	recursionLimit = 10
)

// Resolver represents the aliases resolver.
type Resolver struct {
	// Suffix separator, to perform suffix removal.
	SuffixSep string

	// Characters to drop from the user part.
	DropChars string

	// Path to resolve and exist hooks.
	ExistsHook  string
	ResolveHook string

	// Map of domain -> alias files for that domain.
	// We keep track of them for reloading purposes.
	files   map[string][]string
	domains map[string]bool

	// Map of address -> aliases.
	aliases map[string][]Recipient

	// Map of domain -> catch-all user for the domain
	catchAll map[string]Recipient

	// Mutex protecting the structure.
	mu sync.Mutex
}

// NewResolver returns a new, empty Resolver.
func NewResolver() *Resolver {
	return &Resolver{
		files:    map[string][]string{},
		domains:  map[string]bool{},
		aliases:  map[string][]Recipient{},
		catchAll: map[string]Recipient{},
	}
}

// Resolve the given address, returning the list of corresponding recipients
// (if any).
func (v *Resolver) Resolve(addr string) ([]Recipient, error) {
	return v.resolve(0, addr)
}

// Exists check that the address exists in the database.
// It returns the cleaned address, and a boolean indicating the result.
// The clean address can be used to look it up in other databases, even if it
// doesn't exist.
func (v *Resolver) Exists(addr string) (string, bool) {
	v.mu.Lock()
	addr = v.cleanIfLocal(addr)
	_, ok := v.aliases[addr]
	v.mu.Unlock()

	if ok {
		return addr, true
	}

	return addr, v.runExistsHook(addr)
}

func (v *Resolver) resolve(rcount int, addr string) ([]Recipient, error) {
	if rcount >= recursionLimit {
		return nil, ErrRecursionLimitExceeded
	}

	// Drop suffixes and chars to get the "clean" address before resolving.
	// This also means that we will return the clean version if there's no
	// match, which our callers can rely upon.
	addr = v.cleanIfLocal(addr)

	// Lookup in the aliases database.
	v.mu.Lock()
	rcpts := v.aliases[addr]
	v.mu.Unlock()

	// Augment with the hook results.
	hr, err := v.runResolveHook(addr)
	if err != nil {
		return nil, err
	}
	rcpts = append(rcpts, hr...)

	if len(rcpts) == 0 {
		return []Recipient{{addr, EMAIL}}, nil
	}

	ret := []Recipient{}
	for _, r := range rcpts {
		// Only recurse for email recipients.
		if r.Type != EMAIL {
			ret = append(ret, r)
			continue
		}

		ar, err := v.resolve(rcount+1, r.Addr)
		if err != nil {
			return nil, err
		}

		ret = append(ret, ar...)
	}

	return ret, nil
}

func (v *Resolver) cleanIfLocal(addr string) string {
	user, domain := envelope.Split(addr)

	if !v.domains[domain] {
		return addr
	}

	user = removeAllAfter(user, v.SuffixSep)
	user = removeChars(user, v.DropChars)
	user, _ = normalize.User(user)
	return user + "@" + domain
}

// AddDomain to the resolver, registering its existence.
func (v *Resolver) AddDomain(domain string) {
	v.mu.Lock()
	v.domains[domain] = true
	v.mu.Unlock()
}

// CatchAllAddress returns catch-all address, if such a
// catch-all address for the given domain was set up, otherwise
// an empty string is returned.
func (v *Resolver) CatchAllAddress(domain string) string {
	rcpt, exists := v.catchAll[domain]
	if exists {
		return rcpt.Addr
	}
	return ""
}

// AddAliasesFile to the resolver. The file will be parsed, and an error
// returned if it does not exist or parse correctly.
func (v *Resolver) AddAliasesFile(domain, path string) error {
	// We unconditionally add the domain and file on our list.
	// Even if the file does not exist now, it may later. This makes it be
	// consider when doing Reload.
	// Adding it to the domains mean that we will do drop character and suffix
	// manipulation even if there are no aliases for it.
	v.mu.Lock()
	v.files[domain] = append(v.files[domain], path)
	v.domains[domain] = true
	v.mu.Unlock()

	aliases, err := parseFile(domain, path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// Add the aliases to the resolver, overriding any previous values.
	v.mu.Lock()
	for addr, rs := range aliases {
		if addr != CATCH_ALL_TOKEN {
			v.aliases[addr] = rs
		} else {
			v.catchAll[domain] = rs[0]
		}
	}
	v.mu.Unlock()

	return nil
}

// AddAliasForTesting adds an alias to the resolver, for testing purposes.
// Not for use in production code.
func (v *Resolver) AddAliasForTesting(addr, rcpt string, rType RType) {
	v.aliases[addr] = append(v.aliases[addr], Recipient{rcpt, rType})
}

// Reload aliases files for all known domains.
func (v *Resolver) Reload() error {
	newAliases := map[string][]Recipient{}
	catchAll := map[string]Recipient{}

	for domain, paths := range v.files {
		for _, path := range paths {
			aliases, err := parseFile(domain, path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("error parsing %q: %v", path, err)
			}

			// Add the aliases to the resolver, overriding any previous values.
			for addr, rs := range aliases {
				if addr != CATCH_ALL_TOKEN {
					newAliases[addr] = rs
				} else {
					catchAll[domain] = rs[0]
				}
			}
		}
	}

	v.mu.Lock()
	v.aliases = newAliases
	v.catchAll = catchAll
	v.mu.Unlock()

	return nil
}

func parseFile(domain, path string) (map[string][]Recipient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	aliases, err := parseReader(domain, f)
	if err != nil {
		return nil, fmt.Errorf("reading %q: %v", path, err)
	}
	return aliases, nil
}

func parseReader(domain string, r io.Reader) (map[string][]Recipient, error) {
	aliases := map[string][]Recipient{}

	scanner := bufio.NewScanner(r)
	for i := 1; scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}

		sp := strings.SplitN(line, ":", 2)
		if len(sp) != 2 {
			continue
		}

		addr, rawalias := strings.TrimSpace(sp[0]), strings.TrimSpace(sp[1])
		if len(addr) == 0 || len(rawalias) == 0 {
			continue
		}

		if strings.Contains(addr, "@") {
			// It's invalid for lhs addresses to contain @ (for now).
			continue
		}

		// catch-all parsing
		if addr == CATCH_ALL_TOKEN {
			catchAll := parseRHS(rawalias, domain)
			if catchAll == nil || len(catchAll) != 1 || catchAll[0].Type != EMAIL {
				return nil, fmt.Errorf("catch-all rule may contain exactly one (email) address")
			}
			aliases[CATCH_ALL_TOKEN] = catchAll
			return aliases, nil
		}

		addr = addr + "@" + domain
		addr, _ = normalize.Addr(addr)

		rs := parseRHS(rawalias, domain)
		aliases[addr] = rs
	}

	return aliases, scanner.Err()
}

func parseRHS(rawalias, domain string) []Recipient {
	if len(rawalias) == 0 {
		return nil
	}
	if rawalias[0] == '|' {
		cmd := strings.TrimSpace(rawalias[1:])
		if cmd == "" {
			// A pipe alias without a command is invalid.
			return nil
		}
		return []Recipient{{cmd, PIPE}}
	}

	rs := []Recipient{}
	for _, a := range strings.Split(rawalias, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		// Addresses with no domain get the current one added, so it's
		// easier to share alias files.
		if !strings.Contains(a, "@") {
			a = a + "@" + domain
		}
		a, _ = normalize.Addr(a)
		rs = append(rs, Recipient{a, EMAIL})
	}
	return rs
}

// removeAllAfter removes everything from s that comes after the separators,
// including them.
func removeAllAfter(s, seps string) string {
	for _, c := range strings.Split(seps, "") {
		if c == "" {
			continue
		}

		i := strings.Index(s, c)
		if i == -1 {
			continue
		}

		s = s[:i]
	}

	return s
}

// removeChars removes the runes in "chars" from s.
func removeChars(s, chars string) string {
	for _, c := range strings.Split(chars, "") {
		s = strings.Replace(s, c, "", -1)
	}

	return s
}

func (v *Resolver) runResolveHook(addr string) ([]Recipient, error) {
	if v.ResolveHook == "" {
		hookResults.Add("resolve:notset", 1)
		return nil, nil
	}
	// TODO: check if the file is executable.
	if _, err := os.Stat(v.ResolveHook); os.IsNotExist(err) {
		hookResults.Add("resolve:skip", 1)
		return nil, nil
	}

	// TODO: this should be done via a context propagated all the way through.
	tr := trace.New("Hook.Alias-Resolve", addr)
	defer tr.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, v.ResolveHook, addr)

	outb, err := cmd.Output()
	out := string(outb)
	tr.Debugf("stdout: %q", out)
	if err != nil {
		hookResults.Add("resolve:fail", 1)
		tr.Error(err)
		return nil, err
	}

	// Extract recipients from the output.
	// Same format as the right hand side of aliases file, see parseRHS.
	domain := envelope.DomainOf(addr)
	raw := strings.TrimSpace(out)
	rs := parseRHS(raw, domain)

	tr.Debugf("recipients: %v", rs)
	hookResults.Add("resolve:success", 1)
	return rs, nil
}

func (v *Resolver) runExistsHook(addr string) bool {
	if v.ExistsHook == "" {
		hookResults.Add("exists:notset", 1)
		return false
	}
	// TODO: check if the file is executable.
	if _, err := os.Stat(v.ExistsHook); os.IsNotExist(err) {
		hookResults.Add("exists:skip", 1)
		return false
	}

	// TODO: this should be done via a context propagated all the way through.
	tr := trace.New("Hook.Alias-Exists", addr)
	defer tr.Finish()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, v.ExistsHook, addr)

	out, err := cmd.CombinedOutput()
	tr.Debugf("output: %q", string(out))
	if err != nil {
		tr.Debugf("not exists: %v", err)
		hookResults.Add("exists:false", 1)
		return false
	}

	tr.Debugf("exists")
	hookResults.Add("exists:true", 1)
	return true
}
