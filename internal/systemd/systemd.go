// Package systemd implements utility functions to interact with systemd.
package systemd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var (
	// Error to return when $LISTEN_PID does not refer to us.
	ErrPIDMismatch = errors.New("$LISTEN_PID != our PID")

	// First FD for listeners.
	// It's 3 by definition, but using a variable simplifies testing.
	firstFD = 3
)

// Listeners creates a slice net.Listener from the file descriptors passed
// by systemd, via the LISTEN_FDS environment variable.
// See sd_listen_fds(3) and sd_listen_fds_with_names(3) for more details.
func Listeners() (map[string][]net.Listener, error) {
	pidStr := os.Getenv("LISTEN_PID")
	nfdsStr := os.Getenv("LISTEN_FDS")
	fdNamesStr := os.Getenv("LISTEN_FDNAMES")
	fdNames := strings.Split(fdNamesStr, ":")

	// Nothing to do if the variables are not set.
	if pidStr == "" || nfdsStr == "" {
		return nil, nil
	}

	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, fmt.Errorf(
			"error converting $LISTEN_PID=%q: %v", pidStr, err)
	} else if pid != os.Getpid() {
		return nil, ErrPIDMismatch
	}

	nfds, err := strconv.Atoi(os.Getenv("LISTEN_FDS"))
	if err != nil {
		return nil, fmt.Errorf(
			"error reading $LISTEN_FDS=%q: %v", nfdsStr, err)
	}

	// We should have as many names as we have descriptors.
	// Note that if we have no descriptors, fdNames will be [""] (due to how
	// strings.Split works), so we consider that special case.
	if nfds > 0 && (fdNamesStr == "" || len(fdNames) != nfds) {
		return nil, fmt.Errorf(
			"Incorrect LISTEN_FDNAMES, have you set FileDescriptorName?")
	}

	listeners := map[string][]net.Listener{}

	for i := 0; i < nfds; i++ {
		fd := firstFD + i
		// We don't want childs to inherit these file descriptors.
		syscall.CloseOnExec(fd)

		name := fdNames[i]

		sysName := fmt.Sprintf("[systemd-fd-%d-%v]", fd, name)
		lis, err := net.FileListener(os.NewFile(uintptr(fd), sysName))
		if err != nil {
			return nil, fmt.Errorf(
				"Error making listener out of fd %d: %v", fd, err)
		}

		listeners[name] = append(listeners[name], lis)
	}

	// Remove them from the environment, to prevent accidental reuse (by
	// us or children processes).
	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")

	return listeners, nil
}
