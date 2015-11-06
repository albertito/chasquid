package courier

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"blitiri.com.ar/go/chasquid/internal/trace"
)

var (
	// Location of the procmail binary, and arguments to use.
	// The string "%user%" will be replaced with the local user.
	procmailBin  = "procmail"
	procmailArgs = []string{"-d", "%user%"}

	// Give procmail 1m to deliver mail.
	procmailTimeout = 1 * time.Minute
)

var (
	timeoutError = fmt.Errorf("Operation timed out")
)

// Procmail delivers local mail via procmail.
type Procmail struct {
}

func (p *Procmail) Deliver(from string, to string, data []byte) error {
	tr := trace.New("Procmail", "Deliver")
	defer tr.Finish()

	// Get the user, and sanitize to be extra paranoid.
	user := sanitizeForProcmail(userOf(to))
	tr.LazyPrintf("%s  ->  %s (%s)", from, user, to)

	// Prepare the command, replacing the necessary arguments.
	args := []string{}
	for _, a := range procmailArgs {
		args = append(args, strings.Replace(a, "%user%", user, -1))
	}
	cmd := exec.Command(procmailBin, args...)

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
		return tr.Error(timeoutError)
	}
	if err != nil {
		return tr.Errorf("Procmail failed: %v - %q", err, output.String())
	}
	return nil
}

// sanitizeForProcmail cleans the string, leaving only [a-zA-Z-.].
func sanitizeForProcmail(s string) string {
	valid := func(r rune) rune {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '-', r == '.':
			return r
		default:
			return rune(-1)
		}
	}
	return strings.Map(valid, s)
}
