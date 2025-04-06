package aliases

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"blitiri.com.ar/go/chasquid/internal/trace"
)

type Cases []struct {
	addr   string
	expect []Recipient
	err    error
}

func (cases Cases) check(t *testing.T, r *Resolver) {
	t.Helper()
	tr := trace.New("test", "check")
	defer tr.Finish()

	for _, c := range cases {
		got, err := r.Resolve(tr, c.addr)
		if err != c.err {
			t.Errorf("case %q: expected error %v, got %v",
				c.addr, c.err, err)
		}
		if !reflect.DeepEqual(got, c.expect) {
			t.Errorf("case %q: got %+v, expected %+v",
				c.addr, got, c.expect)
		}
	}
}

func mustExist(t *testing.T, r *Resolver, addrs ...string) {
	t.Helper()
	tr := trace.New("test", "mustExist")
	defer tr.Finish()
	for _, addr := range addrs {
		if ok := r.Exists(tr, addr); !ok {
			t.Errorf("address %q does not exist, it should", addr)
		}
	}
}

func mustNotExist(t *testing.T, r *Resolver, addrs ...string) {
	t.Helper()
	tr := trace.New("test", "mustNotExist")
	defer tr.Finish()
	for _, addr := range addrs {
		if ok := r.Exists(tr, addr); ok {
			t.Errorf("address %q exists, it should not", addr)
		}
	}
}

func allUsersExist(tr *trace.Trace, user, domain string) (bool, error) {
	return true, nil
}

func noUsersExist(tr *trace.Trace, user, domain string) (bool, error) {
	return false, nil
}

func usersWithXDontExist(tr *trace.Trace, user, domain string) (bool, error) {
	if strings.HasPrefix(user, "x") {
		return false, nil
	}
	return true, nil
}

var errUserLookup = errors.New("test error errUserLookup")

func usersWithXErrorYDontExist(tr *trace.Trace, user, domain string) (bool, error) {
	if strings.HasPrefix(user, "x") {
		return false, errUserLookup
	}
	if strings.HasPrefix(user, "y") {
		return false, nil
	}
	return true, nil
}

func email(addr string) Recipient {
	return Recipient{addr, EMAIL}
}

func pipe(addr string) Recipient {
	return Recipient{addr, PIPE}
}

func TestBasic(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("localA")
	resolver.AddDomain("localB")
	resolver.aliases = map[string][]Recipient{
		"a@localA":   {email("c@d"), email("e@localB")},
		"e@localB":   {pipe("cmd")},
		"cmd@localA": {email("x@y")},
	}

	cases := Cases{
		{"a@localA", []Recipient{email("c@d"), pipe("cmd")}, nil},
		{"e@localB", []Recipient{pipe("cmd")}, nil},
		{"x@y", []Recipient{email("x@y")}, nil},
	}
	cases.check(t, resolver)

	mustExist(t, resolver, "a@localA", "e@localB", "cmd@localA")
	mustNotExist(t, resolver, "x@y")
}

func TestCatchAll(t *testing.T) {
	resolver := NewResolver(usersWithXDontExist)
	resolver.DropChars = "."
	resolver.SuffixSep = "+"
	resolver.AddDomain("dom")
	resolver.aliases = map[string][]Recipient{
		"a@dom": {email("a@remote")},
		"b@dom": {email("c@dom")},
		"c@dom": {pipe("cmd")},
		"*@dom": {email("c@dom")},
	}

	cases := Cases{
		{"a@dom", []Recipient{email("a@remote")}, nil},
		{"a+z@dom", []Recipient{email("a@remote")}, nil},
		{"a.@dom", []Recipient{email("a@remote")}, nil},
		{"b@dom", []Recipient{pipe("cmd")}, nil},
		{"c@dom", []Recipient{pipe("cmd")}, nil},
		{"x@dom", []Recipient{pipe("cmd")}, nil},

		// Remote should be returned as-is regardless.
		{"a@remote", []Recipient{email("a@remote")}, nil},
		{"x@remote", []Recipient{email("x@remote")}, nil},
	}
	cases.check(t, resolver)

	mustExist(t, resolver,
		// Exist as users.
		"a@dom", "b@dom", "c@dom",

		// Do not exist as users, but catch-all saves them.
		"x@dom", "x1@dom")
}

func TestRightSideAsterisk(t *testing.T) {
	resolver := NewResolver(noUsersExist)
	resolver.DropChars = "."
	resolver.SuffixSep = "+"
	resolver.AddDomain("dom1")
	resolver.AddDomain("dom2")
	resolver.AddDomain("dom3")
	resolver.AddDomain("dom4")
	resolver.AddDomain("dom5")
	resolver.aliases = map[string][]Recipient{
		"a@dom1": {email("aaa@remote")},

		// Note this goes to dom2 which is local too, and will be resolved
		// recursively.
		"*@dom1": {email("*@dom2")},

		"b@dom2": {email("bbb@remote")},
		"*@dom2": {email("*@remote")},

		// A right hand asterisk on a specific address isn't very useful, but
		// it is supported.
		"z@dom1": {email("*@remote")},

		// Asterisk to asterisk creates an infinite loop.
		"*@dom3": {email("*@dom3")},

		// A right-side asterisk as part of multiple addresses, some of which
		// are fixed.
		"*@dom4": {email("*@remote1"), email("*@remote2"),
			email("fixed@remote3")},

		// A chain of a -> b -> * -> *@remote.
		// This checks which one is used as the "original" user.
		"a@dom5": {email("b@dom5")},
		"*@dom5": {email("*@remote")},
	}

	cases := Cases{
		{"a@dom1", []Recipient{email("aaa@remote")}, nil},
		{"b@dom1", []Recipient{email("bbb@remote")}, nil},
		{"xyz@dom1", []Recipient{email("xyz@remote")}, nil},
		{"xyz@dom2", []Recipient{email("xyz@remote")}, nil},
		{"z@dom1", []Recipient{email("z@remote")}, nil},

		// Check that we match after dropping the characters as needed.
		// This is not specific to the right side asterisk, but serve to
		// confirm we're not matching against it by accident.
		{"a+lala@dom1", []Recipient{email("aaa@remote")}, nil},
		{"a..@dom1", []Recipient{email("aaa@remote")}, nil},

		// Check we don't remove drop characters or suffixes when doing the
		// rewrite: we expect to pass addresses as they come if they didn't
		// match previously.
		{"xyz+abcd@dom1", []Recipient{email("xyz+abcd@remote")}, nil},
		{"x.y.z@dom1", []Recipient{email("x.y.z@remote")}, nil},

		// This one should fail because it creates an infinite loop.
		{"x@dom3", nil, ErrRecursionLimitExceeded},

		// Check the multiple addresses case.
		{"abc@dom4", []Recipient{
			email("abc@remote1"),
			email("abc@remote2"),
			email("fixed@remote3"),
		}, nil},

		// Check the chain case: a -> b -> * -> remote.
		{"a@dom5", []Recipient{email("b@remote")}, nil},
		{"b@dom5", []Recipient{email("b@remote")}, nil},
		{"c@dom5", []Recipient{email("c@remote")}, nil},
	}
	cases.check(t, resolver)
}

func TestUserLookupErrors(t *testing.T) {
	resolver := NewResolver(usersWithXErrorYDontExist)
	resolver.AddDomain("dom")
	resolver.aliases = map[string][]Recipient{
		"a@dom": {email("a@remote")},
		"b@dom": {email("x@dom")},
		"*@dom": {email("x@dom")},
	}

	cases := Cases{
		{"a@dom", []Recipient{email("a@remote")}, nil},
		{"b@dom", nil, errUserLookup},
		{"c@dom", []Recipient{email("c@dom")}, nil},
		{"x@dom", nil, errUserLookup},

		// This one goes through the catch-all.
		{"y@dom", nil, errUserLookup},
	}
	cases.check(t, resolver)
}

func TestAddrRewrite(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("def")
	resolver.AddDomain("p-q.com")
	resolver.aliases = map[string][]Recipient{
		"abc@def":  {email("x@y")},
		"ñoño@def": {email("x@y")},
		"recu@def": {email("ab+cd@p-q.com")},
		"remo@def": {email("x-@y-z.com")},

		// Aliases with a suffix, to make sure we handle them correctly.
		// Note we don't allow aliases with drop characters, they get
		// normalized at parsing time.
		"recu-zzz@def": {email("z@z")},
	}
	resolver.DropChars = ".~"
	resolver.SuffixSep = "-+"

	cases := Cases{
		{"abc@def", []Recipient{email("x@y")}, nil},
		{"a.b.c@def", []Recipient{email("x@y")}, nil},
		{"a~b~c@def", []Recipient{email("x@y")}, nil},
		{"a.b~c@def", []Recipient{email("x@y")}, nil},
		{"abc-ñaca@def", []Recipient{email("x@y")}, nil},
		{"abc-ñaca@def", []Recipient{email("x@y")}, nil},
		{"abc-xyz@def", []Recipient{email("x@y")}, nil},
		{"abc+xyz@def", []Recipient{email("x@y")}, nil},
		{"abc-x.y+z@def", []Recipient{email("x@y")}, nil},

		{"ñ.o~ño-ñaca@def", []Recipient{email("x@y")}, nil},

		// Don't mess with the domain, even if it's known.
		{"a.bc-ñaca@p-q.com", []Recipient{email("abc@p-q.com")}, nil},

		// Clean the right hand side too (if it's a local domain).
		{"recu+blah@def", []Recipient{email("ab@p-q.com")}, nil},

		// Requests for "recu" and variants, because it has an alias with a
		// suffix.
		{"re-cu@def", []Recipient{email("re@def")}, nil},
		{"re.cu@def", []Recipient{email("ab@p-q.com")}, nil},
		{"re.cu-zzz@def", []Recipient{email("z@z")}, nil},

		// Check that because we have an alias with a suffix, we do not
		// accidentally use it for their "clean" versions.
		{"re@def", []Recipient{email("re@def")}, nil},
		{"r.e.c.u@def", []Recipient{email("ab@p-q.com")}, nil},
		{"re.cu-yyy@def", []Recipient{email("ab@p-q.com")}, nil},

		// We should not mess with emails for domains we don't know.
		{"xy@z.com", []Recipient{email("xy@z.com")}, nil},
		{"x.y@z.com", []Recipient{email("x.y@z.com")}, nil},
		{"x-@y-z.com", []Recipient{email("x-@y-z.com")}, nil},
		{"x+blah@y", []Recipient{email("x+blah@y")}, nil},
		{"remo@def", []Recipient{email("x-@y-z.com")}, nil},
	}
	cases.check(t, resolver)
}

func TestExists(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("def")
	resolver.AddDomain("p-q.com")
	resolver.aliases = map[string][]Recipient{
		"abc@def":  {email("x@y")},
		"ñoño@def": {email("x@y")},
		"recu@def": {email("ab+cd@p-q.com")},

		// Aliases with a suffix, to make sure we handle them correctly.
		// Note we don't allow aliases with drop characters, they get
		// normalized at parsing time.
		"ex-act@def": {email("x@y")},
	}
	resolver.DropChars = ".~"
	resolver.SuffixSep = "-+"

	mustExist(t, resolver,
		"abc@def",
		"abc+blah@def",
		"a.bc+blah@def",
		"a.b~c@def",
		"ñoño@def",
		"ño.ño@def",
		"recu@def",
		"re.cu@def",
		"ex-act@def",
	)
	mustNotExist(t, resolver,
		"abc@d.ef",
		"nothere@def",
		"ex@def",
		"a.bc@unknown",
		"x.yz@def",
		"x.yz@d.ef",
		"abc@d.ef",
		"exact@def",
		"exa.ct@def",
		"ex@def",
	)
}

func TestRemoveDropsAndSuffix(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("def")
	resolver.AddDomain("p-q.com")
	resolver.aliases = map[string][]Recipient{
		"abc@def":  {email("x@y")},
		"ñoño@def": {email("x@y")},
		"recu@def": {email("ab+cd@p-q.com")},
	}
	resolver.DropChars = ".~"
	resolver.SuffixSep = "-+"

	cases := []struct {
		addr string
		want string
	}{
		{"abc@def", "abc@def"},
		{"abc+blah@def", "abc@def"},
		{"a.b~c@def", "abc@def"},
		{"a.bc+blah@def", "abc@def"},
		{"x.yz@def", "xyz@def"},
		{"x.yz@d.ef", "xyz@d.ef"},

		// Cases with tricky mixes of suffix separators, to make sure we
		// handle them correctly.
		{"ab-xy@def", "ab@def"},  // Normal.
		{"ab-+xy@def", "ab@def"}, // The two together, in different order.
		{"ab+-xy@def", "ab@def"},
		{"ab-@def", "ab@def"}, // Ending in a separator.
		{"ab+@def", "ab@def"},
		{"ab-+@def", "ab@def"},
		{"ab-xy-z@def", "ab@def"}, // Multiple in different places.
		{"ab+xy-z@def", "ab@def"},
		{"ab-xy+z@def", "ab@def"},
		{"ab--xy@def", "ab@def"}, // Repeated separators.
		{"ab++xy@def", "ab@def"},
		{"ab+-xy@def", "ab@def"},
		{"ab-+xy@def", "ab@def"},
	}
	for _, c := range cases {
		addr := resolver.RemoveDropsAndSuffix(c.addr)
		if addr != c.want {
			t.Errorf("RemoveDropsAndSuffix(%q): want %q, got %q",
				c.addr, c.want, addr)
		}
	}
}

func TestRemoveDropCharacters(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("def")
	resolver.DropChars = "._"
	resolver.SuffixSep = "-+"

	cases := []struct {
		addr string
		want string
	}{
		{"abc@def", "abc@def"},
		{"abc+blah@def", "abc+blah@def"},
		{"a.b@def", "ab@def"},
		{"a.b+c@def", "ab+c@def"},
		{"a.b+c.d@def", "ab+c.d@def"},
		{"a@def", "a@def"},
		{"a+b@def", "a+b@def"},

		// Cases with UTF-8, to make sure we handle indexing correctly.
		{"ñoño@def", "ñoño@def"},
		{"ñoño+blah@def", "ñoño+blah@def"},
		{"ño.ño@def", "ñoño@def"},
		{"ño.ño+blah@def", "ñoño+blah@def"},
		{"ño.ño+ñaca@def", "ñoño+ñaca@def"},
		{"ño.ño+ña.ca@def", "ñoño+ña.ca@def"},
		{"ño.ño+ñaña@def", "ñoño+ñaña@def"},
		{"ño.ño+ña.ña@def", "ñoño+ña.ña@def"},

		// Check "the other" drop char/suffix separator to make sure we
		// don't skip any of them.
		{"a_b@def", "ab@def"},
		{"a_b-c@def", "ab-c@def"},
		{"a_b-c.d@def", "ab-c.d@def"},
		{"ño_ño-ña.ña@def", "ñoño-ña.ña@def"},
	}

	for _, c := range cases {
		addr := resolver.RemoveDropCharacters(c.addr)
		if addr != c.want {
			t.Errorf("RemoveDropCharacters(%q): want %q, got %q",
				c.addr, c.want, addr)
		}
	}
}

func TestTooMuchRecursion(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("b")
	resolver.AddDomain("d")
	resolver.aliases = map[string][]Recipient{
		"a@b": {email("c@d")},
		"c@d": {email("a@b")},
	}

	tr := trace.New("test", "TestTooMuchRecursion")
	defer tr.Finish()

	rs, err := resolver.Resolve(tr, "a@b")
	if err != ErrRecursionLimitExceeded {
		t.Errorf("expected ErrRecursionLimitExceeded, got %v", err)
	}

	if rs != nil {
		t.Errorf("expected nil recipients, got %+v", rs)
	}
}

func TestTooMuchRecursionOnCatchAll(t *testing.T) {
	resolver := NewResolver(usersWithXDontExist)
	resolver.AddDomain("dom")
	resolver.aliases = map[string][]Recipient{
		"a@dom": {email("x@dom")},
		"*@dom": {email("a@dom")},
	}

	cases := Cases{
		// b@dom is local and exists.
		{"b@dom", []Recipient{email("b@dom")}, nil},

		// a@remote is remote.
		{"a@remote", []Recipient{email("a@remote")}, nil},
	}
	cases.check(t, resolver)

	for _, addr := range []string{"a@dom", "x@dom", "xx@dom"} {
		tr := trace.New("TestTooMuchRecursionOnCatchAll", addr)
		defer tr.Finish()

		rs, err := resolver.Resolve(tr, addr)
		if err != ErrRecursionLimitExceeded {
			t.Errorf("%s: expected ErrRecursionLimitExceeded, got %v", addr, err)
		}
		if rs != nil {
			t.Errorf("%s: expected nil recipients, got %+v", addr, rs)
		}
	}
}

func mustWriteFile(t *testing.T, content string) string {
	f, err := os.CreateTemp("", "aliases_test")
	if err != nil {
		t.Fatalf("failed to get temp file: %v", err)
	}
	defer f.Close()

	_, err = f.WriteString(content)
	if err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	return f.Name()
}

func TestAddFile(t *testing.T) {
	cases := []struct {
		contents string
		expected []Recipient
	}{
		{"", []Recipient{email("a@dom")}},
		{"\n\n", []Recipient{email("a@dom")}},
		{" # Comment\n", []Recipient{email("a@dom")}},

		{"a: b\n", []Recipient{email("b@dom")}},
		{"a:b\n", []Recipient{email("b@dom")}},
		{"a : b \n", []Recipient{email("b@dom")}},
		{"a:b,\n", []Recipient{email("b@dom")}},

		{"a: |cmd\n", []Recipient{pipe("cmd")}},
		{"a:|cmd\n", []Recipient{pipe("cmd")}},
		{"a:| cmd \n", []Recipient{pipe("cmd")}},
		{"a  :| cmd \n", []Recipient{pipe("cmd")}},
		{"a: | cmd  arg1 arg2\n", []Recipient{pipe("cmd  arg1 arg2")}},

		{"a: c@d, e@f, g\n",
			[]Recipient{email("c@d"), email("e@f"), email("g@dom")}},
	}

	tr := trace.New("test", "TestAddFile")
	defer tr.Finish()

	for _, c := range cases {
		fname := mustWriteFile(t, c.contents)
		defer os.Remove(fname)

		resolver := NewResolver(allUsersExist)
		_, err := resolver.AddAliasesFile("dom", fname)
		if err != nil {
			t.Fatalf("case %q, error adding file: %v", c.contents, err)
		}

		got, err := resolver.Resolve(tr, "a@dom")
		if err != nil {
			t.Errorf("case %q, got error: %v", c.contents, err)
			continue
		}
		if !reflect.DeepEqual(got, c.expected) {
			t.Errorf("case %q, got %v, expected %v", c.contents, got, c.expected)
		}
	}

	// Error cases.
	errcases := []struct {
		contents string
		expected string
	}{
		{":\n", "line 1: missing address or alias"},
		{"a: \n", "line 1: missing address or alias"},
		{"a:|\n", "right-side: the pipe alias is missing a command"},
		{"a:| \n", "right-side: the pipe alias is missing a command"},
		{"a@dom: b@c \n", "left-side: cannot contain @"},
		{"a", "line 1: missing ':' in line"},
		{"a: x y z\n", "disallowed rune encountered"},
	}

	for _, c := range errcases {
		fname := mustWriteFile(t, c.contents)
		defer os.Remove(fname)

		resolver := NewResolver(allUsersExist)
		_, err := resolver.AddAliasesFile("dom", fname)
		if err == nil {
			t.Errorf("case %q, expected error, got nil (aliases: %v)",
				c.contents, resolver.aliases)
		} else if !strings.Contains(err.Error(), c.expected) {
			t.Errorf("case %q, got error %q, expected it to contain %q",
				c.contents, err.Error(), c.expected)
		}
	}
}

const richFileContents = `
# This is a "complex" alias file, with a few tricky situations.
# It is used in TestRichFile.

# First some valid cases.
a: b
c: d@e, f,
x: | command

# Overrides.
o1: a
o1: b

# Check that we normalize the right hand side.
aA: bB@dom-B

# Test that exact aliases take precedence.
pq: pa
p.q: pb
p.q+r: pc
pq+r: pd
ppp1: p.q+r
ppp2: p.q
ppp3: ppp2

# Finally one to make the file NOT end in \n:
y: z`

func TestRichFile(t *testing.T) {
	fname := mustWriteFile(t, richFileContents)
	defer os.Remove(fname)

	resolver := NewResolver(allUsersExist)
	resolver.DropChars = "."
	resolver.SuffixSep = "+"
	n, err := resolver.AddAliasesFile("dom", fname)
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	if n != 11 {
		t.Fatalf("expected 11 aliases, got %d", n)
	}

	cases := Cases{
		{"a@dom", []Recipient{email("b@dom")}, nil},
		{"c@dom", []Recipient{email("d@e"), email("f@dom")}, nil},
		{"x@dom", []Recipient{pipe("command")}, nil},

		{"o1@dom", []Recipient{email("b@dom")}, nil},

		{"aA@dom", []Recipient{email("bb@dom-b")}, nil},
		{"aa@dom", []Recipient{email("bb@dom-b")}, nil},

		{"pq@dom", []Recipient{email("pb@dom")}, nil},
		{"p.q@dom", []Recipient{email("pb@dom")}, nil},
		{"p.q+r@dom", []Recipient{email("pd@dom")}, nil},
		{"pq+r@dom", []Recipient{email("pd@dom")}, nil},
		{"pq+z@dom", []Recipient{email("pb@dom")}, nil},
		{"p..q@dom", []Recipient{email("pb@dom")}, nil},
		{"p..q+r@dom", []Recipient{email("pd@dom")}, nil},
		{"ppp1@dom", []Recipient{email("pd@dom")}, nil},
		{"ppp2@dom", []Recipient{email("pb@dom")}, nil},
		{"ppp3@dom", []Recipient{email("pb@dom")}, nil},

		{"y@dom", []Recipient{email("z@dom")}, nil},
	}
	cases.check(t, resolver)
}

func TestManyFiles(t *testing.T) {
	files := map[string]string{
		"d1":      mustWriteFile(t, "a: b\nc:d@e"),
		"domain2": mustWriteFile(t, "a: b\nc:d@e"),
		"dom3":    mustWriteFile(t, "x: y, z"),
		"dom4":    mustWriteFile(t, "a: |cmd"),

		// Cross-domain.
		"xd1": mustWriteFile(t, "a: b@xd2"),
		"xd2": mustWriteFile(t, "b: |cmd"),
	}
	for _, fname := range files {
		defer os.Remove(fname)
	}

	resolver := NewResolver(allUsersExist)
	for domain, fname := range files {
		_, err := resolver.AddAliasesFile(domain, fname)
		if err != nil {
			t.Fatalf("failed to add file: %v", err)
		}
	}

	check := func() {
		cases := Cases{
			{"a@d1", []Recipient{email("b@d1")}, nil},
			{"c@d1", []Recipient{email("d@e")}, nil},
			{"x@d1", []Recipient{email("x@d1")}, nil},
			{"a@domain2", []Recipient{email("b@domain2")}, nil},
			{"c@domain2", []Recipient{email("d@e")}, nil},
			{"x@dom3", []Recipient{email("y@dom3"), email("z@dom3")}, nil},
			{"a@dom4", []Recipient{pipe("cmd")}, nil},
			{"a@xd1", []Recipient{pipe("cmd")}, nil},
		}
		cases.check(t, resolver)
	}

	check()

	// Reload, and check again just in case.
	if err := resolver.Reload(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	check()

	// Make one of the files invalid, then reload. Reload should return an
	// error, and leave the resolver unchanged.
	if err := os.WriteFile(files["d1"], []byte("invalid\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	err := resolver.Reload()
	if err == nil {
		t.Fatalf("expected error, got nil")
	} else if !strings.Contains(err.Error(), "line 1: missing ':' in line") {
		t.Fatalf("expected error to contain 'missing :', got %v", err)
	}
	check()
}

func TestHook(t *testing.T) {
	tr := trace.New("TestHook", "test")
	defer tr.Finish()

	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("localA")
	resolver.aliases = map[string][]Recipient{
		"a@localA": {email("c@d")},
	}

	// First check that the test is set up reasonably.
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{email("c@d")}, nil},
	}.check(t, resolver)

	// Test that the empty hook is run correctly.
	resolver.ResolveHook = "testdata/empty-hook.sh"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{email("c@d")}, nil},
	}.check(t, resolver)

	// Test that a normal hook is run correctly.
	resolver.ResolveHook = "testdata/normal-hook.sh"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{
			email("c@d"), // From the internal aliases.
			email("p@q"), // From the hook.
			email("x@y"), // From the hook.
		}, nil},
	}.check(t, resolver)

	// Test that a non-existent hook is ignored.
	resolver.ResolveHook = "testdata/doesnotexist"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{email("c@d")}, nil},
	}.check(t, resolver)

	// Test a hook that returns an invalid alias.
	resolver.ResolveHook = "testdata/invalid-hook.sh"
	mustNotExist(t, resolver, "a@localA")

	rcpts, err := resolver.Resolve(tr, "a@localA")
	if len(rcpts) != 0 {
		t.Errorf("expected no recipients, got %v", rcpts)
	}
	if !strings.Contains(err.Error(), "the pipe alias is missing a command") {
		t.Errorf("expected 'the pipe alias is missing a command', got: %v", err)
	}

	// Now use a resolver that exits with an error.
	resolver.ResolveHook = "testdata/erroring-hook.sh"

	// Check that the hook is run and the error is propagated.
	mustNotExist(t, resolver, "a@localA")
	rcpts, err = resolver.Resolve(tr, "a@localA")
	if len(rcpts) != 0 {
		t.Errorf("expected no recipients, got %v", rcpts)
	}
	execErr := &exec.ExitError{}
	if !errors.As(err, &execErr) {
		t.Errorf("expected *exec.ExitError, got %T - %v", err, err)
	}
}

// Fuzz testing for the parser.
func FuzzReader(f *testing.F) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("domain")
	resolver.DropChars = "."
	resolver.SuffixSep = "-+"
	f.Add([]byte(richFileContents))
	f.Fuzz(func(t *testing.T, data []byte) {
		resolver.parseReader("domain", bytes.NewReader(data))
	})
}
