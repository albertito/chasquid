package queue

import (
	"bytes"
	"net/mail"
	"text/template"
	"time"
)

// Maximum length of the original message to include in the DSN.
// The receiver of the DSN might have a smaller message size than what we
// accepted, so we truncate to a value that should be large enough to be
// useful, but not problematic for modern deployments.
const maxOrigMsgLen = 256 * 1024

// deliveryStatusNotification creates a delivery status notification (DSN) for
// the given item, and puts it in the queue.
//
// References:
// - https://tools.ietf.org/html/rfc3464 (DSN)
// - https://tools.ietf.org/html/rfc6533 (Internationalized DSN)
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

	info.OriginalMessageID = getMessageID(item.Data)

	info.Boundary = <-newID

	buf := &bytes.Buffer{}
	err := dsnTemplate.Execute(buf, info)
	return buf.Bytes(), err
}

func getMessageID(data []byte) string {
	msg, err := mail.ReadMessage(bytes.NewBuffer(data))
	if err != nil {
		return ""
	}
	return msg.Header.Get("Message-ID")
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

	// Message-ID of the original message.
	OriginalMessageID string

	// MIME boundary to use to form the message.
	Boundary string
}

var dsnTemplate = template.Must(
	template.New("dsn").Parse(
		`From: Mail Delivery System <postmaster-dsn@{{.OurDomain}}>
To: <{{.Destination}}>
Subject: Mail delivery failed: returning message to sender
Message-ID: <{{.MessageID}}>
Date: {{.Date}}
In-Reply-To: {{.OriginalMessageID}}
References: {{.OriginalMessageID}}
X-Failed-Recipients: {{range .FailedTo}}{{.}}, {{end}}
Auto-Submitted: auto-replied
MIME-Version: 1.0
Content-Type: multipart/report; report-type=delivery-status;
    boundary="{{.Boundary}}"


--{{.Boundary}}
Content-Type: text/plain; charset="utf-8"
Content-Disposition: inline
Content-Description: Notification
Content-Transfer-Encoding: 8bit

Delivery of your message to the following recipient(s) failed permanently:

  {{range .FailedTo -}} - {{.}}
  {{- end}}

Technical details:
{{- range .FailedRecipients}}
- "{{.Address}}" ({{.Type}}) failed permanently with error:
    {{.LastFailureMessage}}
{{- end}}
{{- range .PendingRecipients}}
- "{{.Address}}" ({{.Type}}) failed repeatedly and timed out, last error:
    {{.LastFailureMessage}}
{{- end}}


--{{.Boundary}}
Content-Type: message/global-delivery-status
Content-Description: Delivery Report
Content-Transfer-Encoding: 8bit

Reporting-MTA: dns; {{.OurDomain}}

{{range .FailedRecipients -}}
Original-Recipient: utf-8; {{.OriginalAddress}}
Final-Recipient: utf-8; {{.Address}}
Action: failed
Status: 5.0.0
Diagnostic-Code: smtp; {{.LastFailureMessage}}
{{end}}
{{range .PendingRecipients -}}
Original-Recipient: utf-8; {{.OriginalAddress}}
Final-Recipient: utf-8; {{.Address}}
Action: failed
Status: 4.0.0
Diagnostic-Code: smtp; {{.LastFailureMessage}}
{{end}}

--{{.Boundary}}
Content-Type: message/rfc822
Content-Description: Undelivered Message
Content-Transfer-Encoding: 8bit

{{.OriginalMessage}}

--{{.Boundary}}--
`))
