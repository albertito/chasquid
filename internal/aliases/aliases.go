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
// theat entire resolution will fail.
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
	"fmt"
	"os"
	"strings"
	"sync"

	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/normalize"
)

// Recipient represents a single recipient, after resolving aliases.
// They don't have any special interface, the callers will do a type switch
// anyway.
type Recipient struct {
	Addr string
	Type RType
}

type RType string

// Valid recipient types.
const (
	EMAIL RType = "(email)"
	PIPE  RType = "(pipe)"
)

var (
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

	// Map of domain -> alias files for that domain.
	// We keep track of them for reloading purposes.
	files   map[string][]string
	domains map[string]bool

	// Map of address -> aliases.
	aliases map[string][]Recipient

	// Mutex protecting the structure.
	mu sync.Mutex
}

func NewResolver() *Resolver {
	return &Resolver{
		files:   map[string][]string{},
		domains: map[string]bool{},
		aliases: map[string][]Recipient{},
	}
}

func (v *Resolver) Resolve(addr string) ([]Recipient, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.resolve(0, addr)
}

// Exists check that the address exists in the database.
// It returns the cleaned address, and a boolean indicating the result.
// The clean address can be used to look it up in other databases, even if it
// doesn't exist.
func (v *Resolver) Exists(addr string) (string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()

	addr = v.cleanIfLocal(addr)
	_, ok := v.aliases[addr]
	return addr, ok
}

func (v *Resolver) resolve(rcount int, addr string) ([]Recipient, error) {
	if rcount >= recursionLimit {
		return nil, ErrRecursionLimitExceeded
	}

	// Drop suffixes and chars to get the "clean" address before resolving.
	// This also means that we will return the clean version if there's no
	// match, which our callers can rely upon.
	addr = v.cleanIfLocal(addr)

	rcpts := v.aliases[addr]
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

func (v *Resolver) AddDomain(domain string) {
	v.mu.Lock()
	v.domains[domain] = true
	v.mu.Unlock()
}

func (v *Resolver) AddAliasesFile(domain, path string) error {
	// We inconditionally add the domain and file on our list.
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
		v.aliases[addr] = rs
	}
	v.mu.Unlock()

	return nil
}

func (v *Resolver) AddAliasForTesting(addr, rcpt string, rType RType) {
	v.aliases[addr] = append(v.aliases[addr], Recipient{rcpt, rType})
}

func (v *Resolver) Reload() error {
	newAliases := map[string][]Recipient{}

	for domain, paths := range v.files {
		for _, path := range paths {
			aliases, err := parseFile(domain, path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return fmt.Errorf("Error parsing %q: %v", path, err)
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

func parseFile(domain, path string) (map[string][]Recipient, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	aliases := map[string][]Recipient{}

	scanner := bufio.NewScanner(f)
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

		addr = addr + "@" + domain
		addr, _ = normalize.Addr(addr)

		if rawalias[0] == '|' {
			cmd := strings.TrimSpace(rawalias[1:])
			aliases[addr] = []Recipient{{cmd, PIPE}}
		} else {
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
				rs = append(rs, Recipient{a, EMAIL})
			}
			aliases[addr] = rs
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %q: %v", path, err)
	}

	return aliases, nil
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
