package queue

import "testing"

func TestDSN(t *testing.T) {
	item := &Item{
		Message: Message{
			ID:   <-newID,
			From: "from@from.org",
			To:   []string{"toto@africa.org", "negra@sosa.org"},
			Rcpt: []*Recipient{
				{"poe@rcpt", Recipient_EMAIL, Recipient_FAILED,
					"oh! horror!"},
				{"newman@rcpt", Recipient_EMAIL, Recipient_FAILED,
					"oh! the humanity!"}},
			Data:     []byte("data ñaca"),
			Hostname: "from.org",
		},
	}

	msg, err := deliveryStatusNotification(item)
	if err != nil {
		t.Error(err)
	}

	t.Log(string(msg))
}
