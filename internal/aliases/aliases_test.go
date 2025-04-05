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

func TestBasic(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("localA")
	resolver.AddDomain("localB")
	resolver.aliases = map[string][]Recipient{
		"a@localA":   {{"c@d", EMAIL}, {"e@localB", EMAIL}},
		"e@localB":   {{"cmd", PIPE}},
		"cmd@localA": {{"x@y", EMAIL}},
	}

	cases := Cases{
		{"a@localA", []Recipient{{"c@d", EMAIL}, {"cmd", PIPE}}, nil},
		{"e@localB", []Recipient{{"cmd", PIPE}}, nil},
		{"x@y", []Recipient{{"x@y", EMAIL}}, nil},
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
		"a@dom": {{"a@remote", EMAIL}},
		"b@dom": {{"c@dom", EMAIL}},
		"c@dom": {{"cmd", PIPE}},
		"*@dom": {{"c@dom", EMAIL}},
	}

	cases := Cases{
		{"a@dom", []Recipient{{"a@remote", EMAIL}}, nil},
		{"a+z@dom", []Recipient{{"a@remote", EMAIL}}, nil},
		{"a.@dom", []Recipient{{"a@remote", EMAIL}}, nil},
		{"b@dom", []Recipient{{"cmd", PIPE}}, nil},
		{"c@dom", []Recipient{{"cmd", PIPE}}, nil},
		{"x@dom", []Recipient{{"cmd", PIPE}}, nil},

		// Remote should be returned as-is regardless.
		{"a@remote", []Recipient{{"a@remote", EMAIL}}, nil},
		{"x@remote", []Recipient{{"x@remote", EMAIL}}, nil},
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
		"a@dom1": {{"aaa@remote", EMAIL}},

		// Note this goes to dom2 which is local too, and will be resolved
		// recursively.
		"*@dom1": {{"*@dom2", EMAIL}},

		"b@dom2": {{"bbb@remote", EMAIL}},
		"*@dom2": {{"*@remote", EMAIL}},

		// A right hand asterisk on a specific address isn't very useful, but
		// it is supported.
		"z@dom1": {{"*@remote", EMAIL}},

		// Asterisk to asterisk creates an infinite loop.
		"*@dom3": {{"*@dom3", EMAIL}},

		// A right-side asterisk as part of multiple addresses, some of which
		// are fixed.
		"*@dom4": {{"*@remote1", EMAIL}, {"*@remote2", EMAIL},
			{"fixed@remote3", EMAIL}},

		// A chain of a -> b -> * -> *@remote.
		// This checks which one is used as the "original" user.
		"a@dom5": {{"b@dom5", EMAIL}},
		"*@dom5": {{"*@remote", EMAIL}},
	}

	cases := Cases{
		{"a@dom1", []Recipient{{"aaa@remote", EMAIL}}, nil},
		{"b@dom1", []Recipient{{"bbb@remote", EMAIL}}, nil},
		{"xyz@dom1", []Recipient{{"xyz@remote", EMAIL}}, nil},
		{"xyz@dom2", []Recipient{{"xyz@remote", EMAIL}}, nil},
		{"z@dom1", []Recipient{{"z@remote", EMAIL}}, nil},

		// Check that we match after dropping the characters as needed.
		// This is not specific to the right side asterisk, but serve to
		// confirm we're not matching against it by accident.
		{"a+lala@dom1", []Recipient{{"aaa@remote", EMAIL}}, nil},
		{"a..@dom1", []Recipient{{"aaa@remote", EMAIL}}, nil},

		// Check we don't remove drop characters or suffixes when doing the
		// rewrite: we expect to pass addresses as they come if they didn't
		// match previously.
		{"xyz+abcd@dom1", []Recipient{{"xyz+abcd@remote", EMAIL}}, nil},
		{"x.y.z@dom1", []Recipient{{"x.y.z@remote", EMAIL}}, nil},

		// This one should fail because it creates an infinite loop.
		{"x@dom3", nil, ErrRecursionLimitExceeded},

		// Check the multiple addresses case.
		{"abc@dom4", []Recipient{
			{"abc@remote1", EMAIL},
			{"abc@remote2", EMAIL},
			{"fixed@remote3", EMAIL},
		}, nil},

		// Check the chain case: a -> b -> * -> remote.
		{"a@dom5", []Recipient{{"b@remote", EMAIL}}, nil},
		{"b@dom5", []Recipient{{"b@remote", EMAIL}}, nil},
		{"c@dom5", []Recipient{{"c@remote", EMAIL}}, nil},
	}
	cases.check(t, resolver)
}

func TestUserLookupErrors(t *testing.T) {
	resolver := NewResolver(usersWithXErrorYDontExist)
	resolver.AddDomain("dom")
	resolver.aliases = map[string][]Recipient{
		"a@dom": {{"a@remote", EMAIL}},
		"b@dom": {{"x@dom", EMAIL}},
		"*@dom": {{"x@dom", EMAIL}},
	}

	cases := Cases{
		{"a@dom", []Recipient{{"a@remote", EMAIL}}, nil},
		{"b@dom", nil, errUserLookup},
		{"c@dom", []Recipient{{"c@dom", EMAIL}}, nil},
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
		"abc@def":  {{"x@y", EMAIL}},
		"ñoño@def": {{"x@y", EMAIL}},
		"recu@def": {{"ab+cd@p-q.com", EMAIL}},
		"remo@def": {{"x-@y-z.com", EMAIL}},

		// Aliases with a suffix, to make sure we handle them correctly.
		// Note we don't allow aliases with drop characters, they get
		// normalized at parsing time.
		"recu-zzz@def": {{"z@z", EMAIL}},
	}
	resolver.DropChars = ".~"
	resolver.SuffixSep = "-+"

	cases := Cases{
		{"abc@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"a.b.c@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"a~b~c@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"a.b~c@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"abc-ñaca@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"abc-ñaca@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"abc-xyz@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"abc+xyz@def", []Recipient{{"x@y", EMAIL}}, nil},
		{"abc-x.y+z@def", []Recipient{{"x@y", EMAIL}}, nil},

		{"ñ.o~ño-ñaca@def", []Recipient{{"x@y", EMAIL}}, nil},

		// Don't mess with the domain, even if it's known.
		{"a.bc-ñaca@p-q.com", []Recipient{{"abc@p-q.com", EMAIL}}, nil},

		// Clean the right hand side too (if it's a local domain).
		{"recu+blah@def", []Recipient{{"ab@p-q.com", EMAIL}}, nil},

		// Requests for "recu" and variants, because it has an alias with a
		// suffix.
		{"re-cu@def", []Recipient{{"re@def", EMAIL}}, nil},
		{"re.cu@def", []Recipient{{"ab@p-q.com", EMAIL}}, nil},
		{"re.cu-zzz@def", []Recipient{{"z@z", EMAIL}}, nil},

		// Check that because we have an alias with a suffix, we do not
		// accidentally use it for their "clean" versions.
		{"re@def", []Recipient{{"re@def", EMAIL}}, nil},
		{"r.e.c.u@def", []Recipient{{"ab@p-q.com", EMAIL}}, nil},
		{"re.cu-yyy@def", []Recipient{{"ab@p-q.com", EMAIL}}, nil},

		// We should not mess with emails for domains we don't know.
		{"xy@z.com", []Recipient{{"xy@z.com", EMAIL}}, nil},
		{"x.y@z.com", []Recipient{{"x.y@z.com", EMAIL}}, nil},
		{"x-@y-z.com", []Recipient{{"x-@y-z.com", EMAIL}}, nil},
		{"x+blah@y", []Recipient{{"x+blah@y", EMAIL}}, nil},
		{"remo@def", []Recipient{{"x-@y-z.com", EMAIL}}, nil},
	}
	cases.check(t, resolver)
}

func TestExists(t *testing.T) {
	resolver := NewResolver(allUsersExist)
	resolver.AddDomain("def")
	resolver.AddDomain("p-q.com")
	resolver.aliases = map[string][]Recipient{
		"abc@def":  {{"x@y", EMAIL}},
		"ñoño@def": {{"x@y", EMAIL}},
		"recu@def": {{"ab+cd@p-q.com", EMAIL}},

		// Aliases with a suffix, to make sure we handle them correctly.
		// Note we don't allow aliases with drop characters, they get
		// normalized at parsing time.
		"ex-act@def": {{"x@y", EMAIL}},
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
		"abc@def":  {{"x@y", EMAIL}},
		"ñoño@def": {{"x@y", EMAIL}},
		"recu@def": {{"ab+cd@p-q.com", EMAIL}},
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
		"a@b": {{"c@d", EMAIL}},
		"c@d": {{"a@b", EMAIL}},
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
		"a@dom": {{"x@dom", EMAIL}},
		"*@dom": {{"a@dom", EMAIL}},
	}

	cases := Cases{
		// b@dom is local and exists.
		{"b@dom", []Recipient{{"b@dom", EMAIL}}, nil},

		// a@remote is remote.
		{"a@remote", []Recipient{{"a@remote", EMAIL}}, nil},
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
		{"", []Recipient{{"a@dom", EMAIL}}},
		{"\n\n", []Recipient{{"a@dom", EMAIL}}},
		{" # Comment\n", []Recipient{{"a@dom", EMAIL}}},

		{"a: b\n", []Recipient{{"b@dom", EMAIL}}},
		{"a:b\n", []Recipient{{"b@dom", EMAIL}}},
		{"a : b \n", []Recipient{{"b@dom", EMAIL}}},
		{"a:b,\n", []Recipient{{"b@dom", EMAIL}}},

		{"a: |cmd\n", []Recipient{{"cmd", PIPE}}},
		{"a:|cmd\n", []Recipient{{"cmd", PIPE}}},
		{"a:| cmd \n", []Recipient{{"cmd", PIPE}}},
		{"a  :| cmd \n", []Recipient{{"cmd", PIPE}}},
		{"a: | cmd  arg1 arg2\n", []Recipient{{"cmd  arg1 arg2", PIPE}}},

		{"a: c@d, e@f, g\n",
			[]Recipient{{"c@d", EMAIL}, {"e@f", EMAIL}, {"g@dom", EMAIL}}},
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
		{"a@dom", []Recipient{{"b@dom", EMAIL}}, nil},
		{"c@dom", []Recipient{{"d@e", EMAIL}, {"f@dom", EMAIL}}, nil},
		{"x@dom", []Recipient{{"command", PIPE}}, nil},

		{"o1@dom", []Recipient{{"b@dom", EMAIL}}, nil},

		{"aA@dom", []Recipient{{"bb@dom-b", EMAIL}}, nil},
		{"aa@dom", []Recipient{{"bb@dom-b", EMAIL}}, nil},

		{"pq@dom", []Recipient{{"pb@dom", EMAIL}}, nil},
		{"p.q@dom", []Recipient{{"pb@dom", EMAIL}}, nil},
		{"p.q+r@dom", []Recipient{{"pd@dom", EMAIL}}, nil},
		{"pq+r@dom", []Recipient{{"pd@dom", EMAIL}}, nil},
		{"pq+z@dom", []Recipient{{"pb@dom", EMAIL}}, nil},
		{"p..q@dom", []Recipient{{"pb@dom", EMAIL}}, nil},
		{"p..q+r@dom", []Recipient{{"pd@dom", EMAIL}}, nil},
		{"ppp1@dom", []Recipient{{"pd@dom", EMAIL}}, nil},
		{"ppp2@dom", []Recipient{{"pb@dom", EMAIL}}, nil},
		{"ppp3@dom", []Recipient{{"pb@dom", EMAIL}}, nil},

		{"y@dom", []Recipient{{"z@dom", EMAIL}}, nil},
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
			{"a@d1", []Recipient{{"b@d1", EMAIL}}, nil},
			{"c@d1", []Recipient{{"d@e", EMAIL}}, nil},
			{"x@d1", []Recipient{{"x@d1", EMAIL}}, nil},
			{"a@domain2", []Recipient{{"b@domain2", EMAIL}}, nil},
			{"c@domain2", []Recipient{{"d@e", EMAIL}}, nil},
			{"x@dom3", []Recipient{{"y@dom3", EMAIL}, {"z@dom3", EMAIL}}, nil},
			{"a@dom4", []Recipient{{"cmd", PIPE}}, nil},
			{"a@xd1", []Recipient{{"cmd", PIPE}}, nil},
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
		"a@localA": {{"c@d", EMAIL}},
	}

	// First check that the test is set up reasonably.
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{{"c@d", EMAIL}}, nil},
	}.check(t, resolver)

	// Test that the empty hook is run correctly.
	resolver.ResolveHook = "testdata/empty-hook.sh"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{{"c@d", EMAIL}}, nil},
	}.check(t, resolver)

	// Test that a normal hook is run correctly.
	resolver.ResolveHook = "testdata/normal-hook.sh"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{
			{"c@d", EMAIL}, // From the internal aliases.
			{"p@q", EMAIL}, // From the hook.
			{"x@y", EMAIL}, // From the hook.
		}, nil},
	}.check(t, resolver)

	// Test that a non-existent hook is ignored.
	resolver.ResolveHook = "testdata/doesnotexist"
	mustExist(t, resolver, "a@localA")
	Cases{
		{"a@localA", []Recipient{{"c@d", EMAIL}}, nil},
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
