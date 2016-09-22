package courier

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestProcmail(t *testing.T) {
	dir, err := ioutil.TempDir("", "test-chasquid-courier")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	p := Procmail{
		Binary:  "tee",
		Args:    []string{dir + "/%to_user%"},
		Timeout: 1 * time.Minute,
	}

	err = p.Deliver("from@x", "to@local", []byte("data"))
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	data, err := ioutil.ReadFile(dir + "/to")
	if err != nil || !bytes.Equal(data, []byte("data")) {
		t.Errorf("Invalid data: %q - %v", string(data), err)
	}
}

func TestProcmailTimeout(t *testing.T) {
	p := Procmail{"/bin/sleep", []string{"1"}, 100 * time.Millisecond}

	err := p.Deliver("from", "to@local", []byte("data"))
	if err != errTimeout {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestProcmailBadCommandLine(t *testing.T) {
	// Non-existent binary.
	p := Procmail{"thisdoesnotexist", nil, 1 * time.Minute}
	err := p.Deliver("from", "to", []byte("data"))
	if err == nil {
		t.Errorf("unexpected success for non-existent binary")
	}

	// Incorrect arguments.
	p = Procmail{"cat", []string{"--fail_unknown_option"}, 1 * time.Minute}
	err = p.Deliver("from", "to", []byte("data"))
	if err == nil {
		t.Errorf("unexpected success for incorrect arguments")
	}
}

func TestSanitize(t *testing.T) {
	cases := []struct{ v, expected string }{
		// These are the same.
		{"thisisfine", "thisisfine"},
		{"ñaca", "ñaca"},
		{"123-456_789", "123-456_789"},
		{"123+456~789", "123+456~789"},

		// These have problematic characters that get dropped.
		{"with spaces", "withspaces"},
		{"with/slash", "withslash"},
		{"quote';andsemicolon", "quoteandsemicolon"},
		{"a;b", "ab"},
		{`"test"`, "test"},

		// Interesting cases taken from
		// http://www.user.uni-hannover.de/nhtcapri/bidirectional-text.html
		// We allow them, they're the same on both sides.
		{"١٩٩٩–١٢–٣١", "١٩٩٩–١٢–٣١"},
		{"موزه‌ها", "موزه\u200cها"},
	}
	for _, c := range cases {
		out := sanitizeForProcmail(c.v)
		if out != c.expected {
			t.Errorf("%q: expected %q, got %q", c.v, c.expected, out)
		}
	}
}
