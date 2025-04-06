// Package aliases implements an email aliases resolver.
//
// The resolver can parse many files for different domains, and perform
// lookups to resolve the aliases.
//
// # File format
//
// It generally follows the traditional aliases format used by sendmail and
// exim.
//
// The file can contain lines of the form:
//
//	user: address, address
//	user: | command
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
// If the user is the string "*", then it is considered a "catch-all alias":
// emails that don't match any known users or other aliases will be sent here.
//
// # Recipients
//
// Recipients can be of different types:
//   - Email: the usual user@domain we all know and love, this is the default.
//   - Pipe: if the right side starts with "| ", the rest of the line specifies
//     a command to pipe the email through.
//     Command and arguments are space separated. No quoting, escaping, or
//     replacements of any kind.
//
// # Lookups
//
// The resolver will perform lookups recursively, until it finds all the final
// recipients.
//
// There are recursion limits to avoid alias loops. If the limit is reached,
// the entire resolution will fail.
//
// # Suffix removal
//
// The resolver can also remove suffixes from emails, and drop characters
// completely. This can be used to turn "user+blah@domain" into "user@domain",
// and "us.er@domain" into "user@domain".
//
// Both are optional, and the characters configurable globally.
//
// There are more complex semantics around handling of drop characters and
// suffixes, see the documentation for more details.
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
	Via  []string // Used when Type == FORWARD.
	Type RType
}

// RType represents a recipient type, see the constants below for valid values.
type RType string

// Valid recipient types.
const (
	EMAIL   RType = "(email)"
	PIPE    RType = "(pipe)"
	FORWARD RType = "(forward)"
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

// Type of the "does this user exist" function", for convenience.
type existsFn func(tr *trace.Trace, user, domain string) (bool, error)

// Resolver represents the aliases resolver.
type Resolver struct {
	// Suffix separator, to perform suffix removal.
	SuffixSep string

	// Characters to drop from the user part.
	DropChars string

	// Path to the resolve hook.
	ResolveHook string

	// Function to check if a user exists in the userdb.
	userExistsInDB existsFn

	// Map of domain -> alias files for that domain.
	// We keep track of them for reloading purposes.
	files   map[string][]string
	domains map[string]bool

	// Map of address -> aliases.
	aliases map[string][]Recipient

	// Mutex protecting the structure.
	mu sync.Mutex
}

// NewResolver returns a new, empty Resolver.
func NewResolver(userExists existsFn) *Resolver {
	return &Resolver{
		files:   map[string][]string{},
		domains: map[string]bool{},
		aliases: map[string][]Recipient{},

		userExistsInDB: userExists,
	}
}

// Resolve the given address, returning the list of corresponding recipients
// (if any).
func (v *Resolver) Resolve(tr *trace.Trace, addr string) ([]Recipient, error) {
	tr = tr.NewChild("Alias.Resolve", addr)
	defer tr.Finish()
	return v.resolve(0, addr, tr)
}

// Exists check that the address exists in the database.  It must only be
// called for local addresses.
func (v *Resolver) Exists(tr *trace.Trace, addr string) bool {
	tr = tr.NewChild("Alias.Exists", addr)
	defer tr.Finish()

	// First, see if there's an exact match in the database.
	// This allows us to have aliases that include suffixes in them, and have
	// them take precedence.
	rcpts, _ := v.lookup(addr, tr)
	if len(rcpts) > 0 {
		return true
	}

	// "Clean" the address, removing drop characters and suffixes, and try
	// again.
	addr = v.RemoveDropsAndSuffix(addr)
	rcpts, _ = v.lookup(addr, tr)
	if len(rcpts) > 0 {
		return true
	}

	domain := envelope.DomainOf(addr)
	catchAll, _ := v.lookup("*@"+domain, tr)
	return len(catchAll) > 0
}

func (v *Resolver) lookup(addr string, tr *trace.Trace) ([]Recipient, error) {
	// Do a lookup in the aliases map. Note we remove drop characters first,
	// which matches what we did at parsing time. Suffixes, if any, are left
	// as-is; that is handled by the callers.
	clean := v.RemoveDropCharacters(addr)
	v.mu.Lock()
	rcpts := v.aliases[clean]
	v.mu.Unlock()

	// Augment with the hook results.
	// Note we use the original address, to give maximum flexibility to the
	// hooks.
	hr, err := v.runResolveHook(tr, addr)
	if err != nil {
		tr.Debugf("lookup(%q) hook error: %v", addr, err)
		return nil, err
	}

	tr.Debugf("lookup(%q) -> %v + %v", addr, rcpts, hr)
	return append(rcpts, hr...), nil
}

func (v *Resolver) resolve(rcount int, addr string, tr *trace.Trace) ([]Recipient, error) {
	tr.Debugf("%d| resolve(%d, %q)", rcount, rcount, addr)
	if rcount >= recursionLimit {
		return nil, ErrRecursionLimitExceeded
	}

	// If the address is not local, we return it as-is, so delivery is
	// attempted against it.
	// Example: an alias that resolves to a non-local address.
	user, domain := envelope.Split(addr)
	if _, ok := v.domains[domain]; !ok {
		tr.Debugf("%d| non-local domain, returning %q", rcount, addr)
		return []Recipient{{addr, nil, EMAIL}}, nil
	}

	// First, see if there's an exact match in the database.
	// This allows us to have aliases that include suffixes in them, and have
	// them take precedence.
	rcpts, err := v.lookup(addr, tr)
	if err != nil {
		tr.Debugf("%d| error in lookup: %v", rcount, err)
		return nil, err
	}

	if len(rcpts) == 0 {
		// Retry after removing drop characters and suffixes.
		// This also means that we will return the clean version if there's no
		// match, which our callers can rely upon.
		addr = v.RemoveDropsAndSuffix(addr)
		rcpts, err = v.lookup(addr, tr)
		if err != nil {
			tr.Debugf("%d| error in lookup: %v", rcount, err)
			return nil, err
		}
	}

	// No alias for this local address.
	if len(rcpts) == 0 {
		tr.Debugf("%d| no alias found", rcount)
		// If the user exists, then use it as-is, no need to recurse further.
		ok, err := v.userExistsInDB(tr, user, domain)
		if err != nil {
			tr.Debugf("%d| error checking if user exists: %v", rcount, err)
			return nil, err
		}
		if ok {
			tr.Debugf("%d| user exists, returning %q", rcount, addr)
			return []Recipient{{addr, nil, EMAIL}}, nil
		}

		catchAll, err := v.lookup("*@"+domain, tr)
		if err != nil {
			tr.Debugf("%d| error in catchall lookup: %v", rcount, err)
			return nil, err
		}
		if len(catchAll) > 0 {
			// If there's a catch-all, then use it and keep resolving
			// recursively (since the catch-all destination could be an
			// alias).
			tr.Debugf("%d| using catch-all: %v", rcount, catchAll)
			rcpts = catchAll
		} else {
			// Otherwise, return the original address unchanged.
			// The caller will handle that situation, and we don't need to
			// invalidate the whole resolution (there could be other valid
			// aliases).
			// The queue will attempt delivery against this local (but
			// evidently non-existing) address, and the courier will emit a
			// clearer failure, reusing the existing codepaths and simplifying
			// the logic.
			tr.Debugf("%d| no catch-all, returning %q", rcount, addr)
			return []Recipient{{addr, nil, EMAIL}}, nil
		}
	}

	ret := []Recipient{}
	for _, r := range rcpts {
		// PIPE recipients get added as-is. No modification, and no recursion.
		if r.Type == PIPE {
			ret = append(ret, r)
			continue
		}

		// If the user of the destination is "*", then we replace it with the
		// original user.
		// This allows for catch-all aliases that forward using the original
		// user, like "*: *@otherdomain".
		if envelope.UserOf(r.Addr) == "*" {
			newAddr := user + "@" + envelope.DomainOf(r.Addr)
			tr.Debugf("%d| replacing %q with %q", rcount, r.Addr, newAddr)
			r.Addr = newAddr
		}

		// Don't recurse FORWARD recipients, since we explicitly want them to
		// be sent through the given servers.
		// Note we do this here and not above so we support the right-side *.
		if r.Type == FORWARD {
			ret = append(ret, r)
			continue
		}

		ar, err := v.resolve(rcount+1, r.Addr, tr)
		if err != nil {
			tr.Debugf("%d| resolve(%q) returned error: %v", rcount, r.Addr, err)
			return nil, err
		}

		ret = append(ret, ar...)
	}

	tr.Debugf("%d| returning %v", rcount, ret)
	return ret, nil
}

// Remove drop characters, but only up to the first suffix separator.
func (v *Resolver) RemoveDropCharacters(addr string) string {
	user, domain := envelope.Split(addr)

	// Remove drop characters up to the first suffix separator.
	firstSuffixSep := strings.IndexAny(user, v.SuffixSep)
	if firstSuffixSep == -1 {
		firstSuffixSep = len(user)
	}

	nu := ""
	for _, c := range user[:firstSuffixSep] {
		if !strings.ContainsRune(v.DropChars, c) {
			nu += string(c)
		}
	}

	// Copy any remaining suffix as-is.
	if firstSuffixSep < len(user) {
		nu += user[firstSuffixSep:]
	}

	nu, _ = normalize.User(nu)
	return nu + "@" + domain
}

func (v *Resolver) RemoveDropsAndSuffix(addr string) string {
	user, domain := envelope.Split(addr)
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

// AddAliasesFile to the resolver. The file will be parsed, and an error
// returned if it does not parse correctly.  Note that the file not existing
// does NOT result in an error.
func (v *Resolver) AddAliasesFile(domain, path string) (int, error) {
	// We unconditionally add the domain and file on our list.
	// Even if the file does not exist now, it may later. This makes it be
	// consider when doing Reload.
	// Adding it to the domains mean that we will do drop character and suffix
	// manipulation even if there are no aliases for it.
	v.mu.Lock()
	v.files[domain] = append(v.files[domain], path)
	v.domains[domain] = true
	v.mu.Unlock()

	aliases, err := v.parseFile(domain, path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	// Add the aliases to the resolver, overriding any previous values.
	v.mu.Lock()
	for addr, rs := range aliases {
		v.aliases[addr] = rs
	}
	v.mu.Unlock()

	return len(aliases), nil
}

// AddAliasForTesting adds an alias to the resolver, for testing purposes.
// Not for use in production code.
func (v *Resolver) AddAliasForTesting(addr, rcpt string, via []string, rType RType) {
	v.aliases[addr] = append(v.aliases[addr], Recipient{rcpt, via, rType})
}

// Reload aliases files for all known domains.
func (v *Resolver) Reload() error {
	newAliases := map[string][]Recipient{}

	for domain, paths := range v.files {
		for _, path := range paths {
			aliases, err := v.parseFile(domain, path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}

			// Add the aliases to the resolver, overriding any previous values.
			for addr, rs := range aliases {
				newAliases[addr] = rs
			}
		}
	}

	v.mu.Lock()
	v.aliases = newAliases
	v.mu.Unlock()

	return nil
}

func (v *Resolver) parseFile(domain, path string) (map[string][]Recipient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	aliases, err := v.parseReader(domain, f)
	if err != nil {
		return nil, fmt.Errorf("%q %w", path, err)
	}
	return aliases, nil
}

func (v *Resolver) parseReader(domain string, r io.Reader) (map[string][]Recipient, error) {
	aliases := map[string][]Recipient{}

	scanner := bufio.NewScanner(r)
	for i := 1; scanner.Scan(); i++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		sp := strings.SplitN(line, ":", 2)
		if len(sp) != 2 {
			return nil, newParseError(i, "missing ':' in line")
		}

		addr, rawalias := strings.TrimSpace(sp[0]), strings.TrimSpace(sp[1])
		if len(addr) == 0 || len(rawalias) == 0 {
			return nil, newParseError(i, "missing address or alias")
		}

		if strings.Contains(addr, "@") {
			// It's invalid for lhs addresses to contain @ (for now).
			return nil, newParseError(i, "left-side: cannot contain @")
		}

		// We remove DropChars from the address, but leave the suffixes (if
		// any). This matches the behaviour expected by Exists and Resolve,
		// see the documentation for more details.
		addr = addr + "@" + domain
		addr = v.RemoveDropCharacters(addr)
		addr, _ = normalize.Addr(addr)

		rs, err := parseRHS(rawalias, domain)
		if err != nil {
			return nil, newParseError(i, "right-side: %w", err)
		}
		aliases[addr] = rs
	}

	return aliases, scanner.Err()
}

func parseRHS(rawalias, domain string) ([]Recipient, error) {
	var err error
	if len(rawalias) == 0 {
		// Explicitly allow empty rawalias strings at this point: the file
		// parsing will prevent this at the upper level, and when we parse the
		// RHS for the hook invocation, an empty string is valid.
		return nil, nil
	}
	if rawalias[0] == '|' {
		cmd := strings.TrimSpace(rawalias[1:])
		if cmd == "" {
			// A pipe alias without a command is invalid.
			return nil, fmt.Errorf("the pipe alias is missing a command")
		}
		return []Recipient{{cmd, nil, PIPE}}, nil
	}

	rs := []Recipient{}
	for _, a := range strings.Split(rawalias, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			// Ignore empty aliases. This is out of convenience, so we allow
			// things like "a: b,".
			continue
		}

		r := Recipient{
			Addr: a,
			Via:  nil,
			Type: EMAIL,
		}
		if strings.Contains(a, " via ") {
			// It's a FORWARD, so extract the Via part and continue.
			r.Type = FORWARD
			r.Addr, r.Via, err = parseForward(a)
			if err != nil {
				return nil, err
			}
		}

		// Addresses with no domain get the current one added, so it's
		// easier to share alias files. Note we also do this for FORWARD
		// aliases.
		if !strings.Contains(r.Addr, "@") {
			r.Addr = r.Addr + "@" + domain
		}

		r.Addr, err = normalize.Addr(r.Addr)
		if err != nil {
			return nil, fmt.Errorf("normalizing address %q: %w", a, err)
		}

		rs = append(rs, r)
	}
	return rs, nil
}

// Parse a raw FORWARD alias, returning the address and the via part.
// Expected format is "address via server1/server2/server3".
func parseForward(rawalias string) (string, []string, error) {
	// Split the alias into the address and the via part.
	addr, viaS, found := strings.Cut(rawalias, " via ")
	if !found {
		return "", nil, fmt.Errorf("via separator not found")
	}

	// No need to normalize the address, the caller will do it.
	// We only trim it, and enforce that there is one.
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", nil, fmt.Errorf("forwarding alias is missing the address")
	}

	// The via part is a list of servers, separated by "/". Split it up.
	// For now we don't validate the servers, but we may in the future.
	via := []string{}
	for _, v := range strings.Split(viaS, "/") {
		server := strings.TrimSpace(v)
		if server == "" {
			return "", nil, fmt.Errorf("empty server in via list")
		}
		via = append(via, server)
	}

	return addr, via, nil
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

func (v *Resolver) runResolveHook(tr *trace.Trace, addr string) ([]Recipient, error) {
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
	tr = tr.NewChild("Hook.Alias-Resolve", addr)
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
	rs, err := parseRHS(raw, domain)
	if err != nil {
		hookResults.Add("resolve:parse_err", 1)
		tr.Errorf("error parsing hook output: %v", err)
		return nil, err
	}

	tr.Debugf("recipients: %v", rs)
	hookResults.Add("resolve:success", 1)
	return rs, nil
}

type parseError struct {
	line int
	err  error
}

func (e *parseError) Error() string {
	return fmt.Sprintf("line %d: %v", e.line, e.err)
}

func newParseError(line int, f string, args ...interface{}) error {
	return &parseError{
		line: line,
		err:  fmt.Errorf(f, args...),
	}
}
