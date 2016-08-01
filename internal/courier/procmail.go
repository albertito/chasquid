package courier

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode"

	"blitiri.com.ar/go/chasquid/internal/envelope"
	"blitiri.com.ar/go/chasquid/internal/trace"
)

var (
	// Location of the procmail binary, and arguments to use.
	// The string "%user%" will be replaced with the local user.
	// TODO: Make these a part of the courier instance itself? Why do they
	// have to be global?
	MailDeliveryAgentBin  = "procmail"
	MailDeliveryAgentArgs = []string{"-d", "%user%"}

	// Give procmail 1m to deliver mail.
	procmailTimeout = 1 * time.Minute
)

var (
	errTimeout = fmt.Errorf("Operation timed out")
)

// Procmail delivers local mail via procmail.
type Procmail struct {
}

func (p *Procmail) Deliver(from string, to string, data []byte) error {
	tr := trace.New("Procmail", "Deliver")
	defer tr.Finish()

	// Get the user, and sanitize to be extra paranoid.
	user := sanitizeForProcmail(envelope.UserOf(to))
	domain := sanitizeForProcmail(envelope.DomainOf(to))
	tr.LazyPrintf("%s  ->  %s (%s @ %s)", from, user, to, domain)

	// Prepare the command, replacing the necessary arguments.
	replacer := strings.NewReplacer(
		"%user%", user,
		"%domain%", domain)
	args := []string{}
	for _, a := range MailDeliveryAgentArgs {
		args = append(args, replacer.Replace(a))
	}
	cmd := exec.Command(MailDeliveryAgentBin, args...)

	cmdStdin, err := cmd.StdinPipe()
	if err != nil {
		return tr.Errorf("StdinPipe: %v", err)
	}

	output := &bytes.Buffer{}
	cmd.Stdout = output
	cmd.Stderr = output

	err = cmd.Start()
	if err != nil {
		return tr.Errorf("Error starting procmail: %v", err)
	}

	_, err = bytes.NewBuffer(data).WriteTo(cmdStdin)
	if err != nil {
		return tr.Errorf("Error sending data to procmail: %v", err)
	}

	cmdStdin.Close()

	timer := time.AfterFunc(procmailTimeout, func() {
		cmd.Process.Kill()
	})
	err = cmd.Wait()
	timedOut := !timer.Stop()

	if timedOut {
		return tr.Error(errTimeout)
	}
	if err != nil {
		return tr.Errorf("Procmail failed: %v - %q", err, output.String())
	}
	return nil
}

// sanitizeForProcmail cleans the string, removing characters that could be
// problematic considering we will run an external command.
//
// The server does not rely on this to do substitution or proper filtering,
// that's done at a different layer; this is just for defense in depth.
func sanitizeForProcmail(s string) string {
	valid := func(r rune) rune {
		switch {
		case unicode.IsSpace(r), unicode.IsControl(r),
			strings.ContainsRune("/;\"'\\|*&$%()[]{}`!", r):
			return rune(-1)
		default:
			return r
		}
	}
	return strings.Map(valid, s)
}
