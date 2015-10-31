// Package systemd implements utility functions to interact with systemd.
package systemd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
)

var (
	// Error to return when $LISTEN_PID does not refer to us.
	PIDMismatch = errors.New("$LISTEN_PID != our PID")

	// First FD for listeners.
	// It's 3 by definition, but using a variable simplifies testing.
	firstFD = 3
)

// Listeners creates a slice net.Listener from the file descriptors passed
// by systemd, via the LISTEN_FDS environment variable.
// See sd_listen_fds(3) for more details.
func Listeners() ([]net.Listener, error) {
	pidStr := os.Getenv("LISTEN_PID")
	nfdsStr := os.Getenv("LISTEN_FDS")

	// Nothing to do if the variables are not set.
	if pidStr == "" || nfdsStr == "" {
		return nil, nil
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, fmt.Errorf(
			"error converting $LISTEN_PID=%q: %v", pidStr, err)
	} else if pid != os.Getpid() {
		return nil, PIDMismatch
	}

	nfds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil {
		return nil, fmt.Errorf(
			"error reading $LISTEN_FDS=%q: %v", nfdsStr, err)
	}

	listeners := []net.Listener{}

	for fd := firstFD; fd < firstFD+nfds; fd++ {
		// We don't want childs to inherit these file descriptors.
		syscall.CloseOnExec(fd)

		name := fmt.Sprintf("[systemd-fd-%d]", fd)
		lis, err := net.FileListener(os.NewFile(uintptr(fd), name))
		if err != nil {
			return nil, fmt.Errorf(
				"Error making listener out of fd %d: %v", fd, err)
		}

		listeners = append(listeners, lis)
	}

	// Remove them from the environment, to prevent accidental reuse (by
	// us or children processes).
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")

	return listeners, nil
}
