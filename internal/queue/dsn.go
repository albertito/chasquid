package queue

import (
	"bytes"
	"text/template"
	"time"
)

// Maximum length of the original message to include in the DSN.
const maxOrigMsgLen = 256 * 1024

// deliveryStatusNotification creates a delivery status notification (DSN) for
// the given item, and puts it in the queue.
//
// There is a standard, https://tools.ietf.org/html/rfc3464, although most
// MTAs seem to use a plain email and include an X-Failed-Recipients header.
// We're going with the latter for now, may extend it to the former later.
func deliveryStatusNotification(domainFrom string, item *Item) ([]byte, error) {
	info := dsnInfo{
		OurDomain:   domainFrom,
		Destination: item.From,
		MessageID:   "chasquid-dsn-" + <-newID + "@" + domainFrom,
		Date:        time.Now().Format(time.RFC1123Z),
		To:          item.To,
		Recipients:  item.Rcpt,
		FailedTo:    map[string]string{},
	}

	for _, rcpt := range item.Rcpt {
		if rcpt.Status != Recipient_SENT {
			info.FailedTo[rcpt.OriginalAddress] = rcpt.OriginalAddress
			switch rcpt.Status {
			case Recipient_FAILED:
				info.FailedRecipients = append(info.FailedRecipients, rcpt)
			case Recipient_PENDING:
				info.PendingRecipients = append(info.PendingRecipients, rcpt)
			}
		}
	}

	if len(item.Data) > maxOrigMsgLen {
		info.OriginalMessage = string(item.Data[:maxOrigMsgLen])
	} else {
		info.OriginalMessage = string(item.Data)
	}

	buf := &bytes.Buffer{}
	err := dsnTemplate.Execute(buf, info)
	return buf.Bytes(), err
}

type dsnInfo struct {
	OurDomain         string
	Destination       string
	MessageID         string
	Date              string
	To                []string
	FailedTo          map[string]string
	Recipients        []*Recipient
	FailedRecipients  []*Recipient
	PendingRecipients []*Recipient
	OriginalMessage   string
}

var dsnTemplate = template.Must(template.New("dsn").Parse(
	`From: Mail Delivery System <postmaster-dsn@{{.OurDomain}}>
To: <{{.Destination}}>
Subject: Mail delivery failed: returning message to sender
Message-ID: <{{.MessageID}}>
Date: {{.Date}}
X-Failed-Recipients: {{range .FailedTo}}{{.}}, {{end}}
Auto-Submitted: auto-replied

Delivery to the following recipient(s) failed permanently:

  {{range .FailedTo -}} - {{.}}
  {{- end}}


----- Technical details -----
{{range .FailedRecipients}}
- "{{.Address}}" ({{.Type}}) failed permanently with error:
    {{.LastFailureMessage}}
{{end}}
{{- range .PendingRecipients}}
- "{{.Address}}" ({{.Type}}) failed repeatedly and timed out, last error:
    {{.LastFailureMessage}}
{{end}}

----- Original message -----

{{.OriginalMessage}}

`))
