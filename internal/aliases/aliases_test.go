package aliases

import (
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

type Cases []struct {
	addr   string
	expect []Recipient
}

func (cases Cases) check(t *testing.T, r *Resolver) {
	for _, c := range cases {
		got, err := r.Resolve(c.addr)
		if err != nil {
			t.Errorf("case %q, got error: %v", c.addr, err)
			continue
		}
		if !reflect.DeepEqual(got, c.expect) {
			t.Errorf("case %q, got %+v, expected %+v", c.addr, got, c.expect)
		}
	}
}

func TestBasic(t *testing.T) {
	resolver := NewResolver()
	resolver.aliases = map[string][]Recipient{
		"a@b": {{"c@d", EMAIL}, {"e@f", EMAIL}},
		"e@f": {{"cmd", PIPE}},
		"cmd": {{"x@y", EMAIL}}, // it's a trap!
	}

	cases := Cases{
		{"a@b", []Recipient{{"c@d", EMAIL}, {"cmd", PIPE}}},
		{"e@f", []Recipient{{"cmd", PIPE}}},
		{"x@y", []Recipient{{"x@y", EMAIL}}},
	}
	cases.check(t, resolver)
}

func TestAddrRewrite(t *testing.T) {
	resolver := NewResolver()
	resolver.AddDomain("def")
	resolver.AddDomain("p-q.com")
	resolver.aliases = map[string][]Recipient{
		"abc@def":  {{"x@y", EMAIL}},
		"ñoño@def": {{"x@y", EMAIL}},
		"recu@def": {{"ab+cd@p-q.com", EMAIL}},
	}
	resolver.DropChars = ".~"
	resolver.SuffixSep = "-+"

	cases := Cases{
		{"abc@def", []Recipient{{"x@y", EMAIL}}},
		{"a.b.c@def", []Recipient{{"x@y", EMAIL}}},
		{"a~b~c@def", []Recipient{{"x@y", EMAIL}}},
		{"a.b~c@def", []Recipient{{"x@y", EMAIL}}},
		{"abc-ñaca@def", []Recipient{{"x@y", EMAIL}}},
		{"abc-ñaca@def", []Recipient{{"x@y", EMAIL}}},
		{"abc-xyz@def", []Recipient{{"x@y", EMAIL}}},
		{"abc+xyz@def", []Recipient{{"x@y", EMAIL}}},
		{"abc-x.y+z@def", []Recipient{{"x@y", EMAIL}}},

		{"ñ.o~ño-ñaca@def", []Recipient{{"x@y", EMAIL}}},

		// Don't mess with the domain, even if it's known.
		{"a.bc-ñaca@p-q.com", []Recipient{{"abc@p-q.com", EMAIL}}},

		// Clean the right hand side too (if it's a local domain).
		{"recu+blah@def", []Recipient{{"ab@p-q.com", EMAIL}}},

		// We should not mess with emails for domains we don't know.
		{"xy@z.com", []Recipient{{"xy@z.com", EMAIL}}},
		{"x.y@z.com", []Recipient{{"x.y@z.com", EMAIL}}},
		{"x-@y-z.com", []Recipient{{"x-@y-z.com", EMAIL}}},
		{"x+blah@y", []Recipient{{"x+blah@y", EMAIL}}},
	}
	cases.check(t, resolver)
}

func TestTooMuchRecursion(t *testing.T) {
	resolver := Resolver{}
	resolver.aliases = map[string][]Recipient{
		"a@b": {{"c@d", EMAIL}},
		"c@d": {{"a@b", EMAIL}},
	}

	rs, err := resolver.Resolve("a@b")
	if err != ErrRecursionLimitExceeded {
		t.Errorf("expected ErrRecursionLimitExceeded, got %v", err)
	}

	if rs != nil {
		t.Errorf("expected nil recipients, got %+v", rs)
	}
}

func mustWriteFile(t *testing.T, content string) string {
	f, err := ioutil.TempFile("", "aliases_test")
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
		{"\n", []Recipient{{"a@dom", EMAIL}}},
		{" # Comment\n", []Recipient{{"a@dom", EMAIL}}},
		{":\n", []Recipient{{"a@dom", EMAIL}}},
		{"a: \n", []Recipient{{"a@dom", EMAIL}}},
		{"a@dom: b@c \n", []Recipient{{"a@dom", EMAIL}}},

		{"a: b\n", []Recipient{{"b@dom", EMAIL}}},
		{"a:b\n", []Recipient{{"b@dom", EMAIL}}},
		{"a : b \n", []Recipient{{"b@dom", EMAIL}}},
		{"a : b, \n", []Recipient{{"b@dom", EMAIL}}},

		{"a: |cmd\n", []Recipient{{"cmd", PIPE}}},
		{"a:|cmd\n", []Recipient{{"cmd", PIPE}}},
		{"a:| cmd \n", []Recipient{{"cmd", PIPE}}},
		{"a  :| cmd \n", []Recipient{{"cmd", PIPE}}},
		{"a: | cmd  arg1 arg2\n", []Recipient{{"cmd  arg1 arg2", PIPE}}},

		{"a: c@d, e@f, g\n",
			[]Recipient{{"c@d", EMAIL}, {"e@f", EMAIL}, {"g@dom", EMAIL}}},
	}

	for _, c := range cases {
		fname := mustWriteFile(t, c.contents)
		defer os.Remove(fname)

		resolver := NewResolver()
		err := resolver.AddAliasesFile("dom", fname)
		if err != nil {
			t.Fatalf("error adding file: %v", err)
		}

		got, err := resolver.Resolve("a@dom")
		if err != nil {
			t.Errorf("case %q, got error: %v", c.contents, err)
			continue
		}
		if !reflect.DeepEqual(got, c.expected) {
			t.Errorf("case %q, got %v, expected %v", c.contents, got, c.expected)
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

# The following is invalid, should be ignored.
a@dom: x@dom

# Overrides.
o1: a
o1: b

# Finally one to make the file NOT end in \n:
y: z`

func TestRichFile(t *testing.T) {
	fname := mustWriteFile(t, richFileContents)
	defer os.Remove(fname)

	resolver := NewResolver()
	err := resolver.AddAliasesFile("dom", fname)
	if err != nil {
		t.Fatalf("failed to add file: %v", err)
	}

	cases := Cases{
		{"a@dom", []Recipient{{"b@dom", EMAIL}}},
		{"c@dom", []Recipient{{"d@e", EMAIL}, {"f@dom", EMAIL}}},
		{"x@dom", []Recipient{{"command", PIPE}}},
		{"o1@dom", []Recipient{{"b@dom", EMAIL}}},
		{"y@dom", []Recipient{{"z@dom", EMAIL}}},
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

	resolver := NewResolver()
	for domain, fname := range files {
		err := resolver.AddAliasesFile(domain, fname)
		if err != nil {
			t.Fatalf("failed to add file: %v", err)
		}
	}

	check := func() {
		cases := Cases{
			{"a@d1", []Recipient{{"b@d1", EMAIL}}},
			{"c@d1", []Recipient{{"d@e", EMAIL}}},
			{"x@d1", []Recipient{{"x@d1", EMAIL}}},
			{"a@domain2", []Recipient{{"b@domain2", EMAIL}}},
			{"c@domain2", []Recipient{{"d@e", EMAIL}}},
			{"x@dom3", []Recipient{{"y@dom3", EMAIL}, {"z@dom3", EMAIL}}},
			{"a@dom4", []Recipient{{"cmd", PIPE}}},
			{"a@xd1", []Recipient{{"cmd", PIPE}}},
		}
		cases.check(t, resolver)
	}

	check()

	// Reload, and check again just in case.
	if err := resolver.Reload(); err != nil {
		t.Fatalf("failed to reload: %v", err)
	}

	check()
}
