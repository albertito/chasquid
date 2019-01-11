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
			To:   []string{"ñaca@africa.org", "negra@sosa.org"},
			Rcpt: []*Recipient{
				{"poe@rcpt", Recipient_EMAIL, Recipient_FAILED,
					"oh! horror!", "ñaca@africa.org"},
				{"newman@rcpt", Recipient_EMAIL, Recipient_PENDING,
					"oh! the humanity!", "ñaca@africa.org"},
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
In-Reply-To: *
References: *
X-Failed-Recipients: ñaca@africa.org, 
Auto-Submitted: auto-replied
MIME-Version: 1.0
Content-Type: multipart/report; report-type=delivery-status;
    boundary="???????????"


--???????????
Content-Type: text/plain; charset="utf-8"
Content-Disposition: inline
Content-Description: Notification
Content-Transfer-Encoding: 8bit

Delivery of your message to the following recipient(s) failed permanently:

  - ñaca@africa.org

Technical details:
- "poe@rcpt" (EMAIL) failed permanently with error:
    oh! horror!
- "newman@rcpt" (EMAIL) failed repeatedly and timed out, last error:
    oh! the humanity!


--???????????
Content-Type: message/global-delivery-status
Content-Description: Delivery Report
Content-Transfer-Encoding: 8bit

Reporting-MTA: dns; dsnDomain

Original-Recipient: utf-8; ñaca@africa.org
Final-Recipient: utf-8; poe@rcpt
Action: failed
Status: 5.0.0
Diagnostic-Code: smtp; oh! horror!

Original-Recipient: utf-8; ñaca@africa.org
Final-Recipient: utf-8; newman@rcpt
Action: failed
Status: 4.0.0
Diagnostic-Code: smtp; oh! the humanity!


--???????????
Content-Type: message/rfc822
Content-Description: Undelivered Message
Content-Transfer-Encoding: 8bit

data ñaca

--???????????--
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
