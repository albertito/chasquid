package queue

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestDSN(t *testing.T) {
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from@from.org",
			To:   []string{"toto@africa.org", "negra@sosa.org"},
			Rcpt: []*Recipient{
				{"poe@rcpt", Recipient_EMAIL, Recipient_FAILED,
					"oh! horror!", "toto@africa.org"},
				{"newman@rcpt", Recipient_EMAIL, Recipient_PENDING,
					"oh! the humanity!", "toto@africa.org"},
				{"ant@rcpt", Recipient_EMAIL, Recipient_SENT,
					"", "negra@sosa.org"},
			},
			Data: []byte("data ñaca"),
		},
	}

	msg, err := deliveryStatusNotification("dsnDomain", item)
	if err != nil {
		t.Error(err)
	}
	if !flexibleEq(expectedDSN, string(msg)) {
		t.Errorf("generated DSN different than expected")
		printDiff(func(s string) { t.Error(s) }, expectedDSN, string(msg))
	} else {
		t.Log(string(msg))
	}
}

const expectedDSN = `From: Mail Delivery System <postmaster-dsn@dsnDomain>
To: <from@from.org>
Subject: Mail delivery failed: returning message to sender
Message-ID: <chasquid-dsn-???????????@dsnDomain>
Date: *
X-Failed-Recipients: toto@africa.org, 
Auto-Submitted: auto-replied

Delivery to the following recipient(s) failed permanently:

  - toto@africa.org


----- Technical details -----

- "poe@rcpt" (EMAIL) failed permanently with error:
    oh! horror!

- "newman@rcpt" (EMAIL) failed repeatedly and timed out, last error:
    oh! the humanity!


----- Original message -----

data ñaca

`

// flexibleEq compares two strings, supporting wildcards.
// Not particularly nice or robust, only useful for testing.
func flexibleEq(expected, got string) bool {
	posG := 0
	for i := 0; i < len(expected); i++ {
		if posG >= len(got) {
			return false
		}

		c := expected[i]
		if c == '?' {
			posG++
			continue
		} else if c == '*' {
			for got[posG] != '\n' {
				posG++
			}
			continue
		} else if byte(c) != got[posG] {
			return false
		}

		posG++
	}

	return true
}

// printDiff prints the difference between the strings using the given
// function.  This is a _horrible_ implementation, only useful for testing.
func printDiff(print func(s string), expected, got string) {
	lines := []string{}

	// expected lines and map.
	eM := map[string]int{}
	for _, l := range strings.Split(expected, "\n") {
		eM[l]++
		lines = append(lines, l)
	}

	// got lines and map.
	gM := map[string]int{}
	for _, l := range strings.Split(got, "\n") {
		gM[l]++
		lines = append(lines, l)
	}

	// sort the lines, to make it easier to see the differences (this works
	// ok when there's few, horrible when there's lots).
	sort.Strings(lines)

	// print diff of expected vs. got
	seen := map[string]bool{}
	print("E  G  | Line")
	for _, l := range lines {
		if !seen[l] && eM[l] != gM[l] {
			print(fmt.Sprintf("%2d %2d | %q", eM[l], gM[l], l))
			seen[l] = true
		}
	}
}
