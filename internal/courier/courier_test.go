package courier

import "testing"

// Counter courier, for testing purposes.
type counter struct {
	c int
}

func (c *counter) Deliver(from string, to string, data []byte) error {
	c.c++
	return nil
}

func TestRouter(t *testing.T) {
	localC := &counter{}
	remoteC := &counter{}
	r := Router{
		Local:  localC,
		Remote: remoteC,
		LocalDomains: map[string]bool{
			"local1": true,
			"local2": true,
		},
	}

	for domain, c := range map[string]int{
		"local1": 1,
		"local2": 2,
		"remote": 9,
	} {
		for i := 0; i < c; i++ {
			r.Deliver("from", "a@"+domain, nil)
		}
	}

	if localC.c != 3 {
		t.Errorf("local mis-count: expected 3, got %d", localC.c)
	}

	if remoteC.c != 9 {
		t.Errorf("remote mis-count: expected 9, got %d", remoteC.c)
	}
}
