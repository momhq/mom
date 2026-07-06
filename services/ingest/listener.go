package ingest

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"syscall"
)

// DefaultPort is the default TCP port for the ingest server.
const DefaultPort = 7475

// ListenWithFallback binds a TCP listener on host:preferred. If the port is
// taken, it tries up to attempts consecutive ports (preferred+1, preferred+2, …).
// attempts=0 disables fallback (use when the user explicitly chose the port).
//
// An empty host defaults to "localhost" (loopback-only bind). Binding on all
// interfaces would expose company-record events to anyone on the network; this
// default matches the lens listener pattern (ADR 0025).
func ListenWithFallback(host string, preferred, attempts int) (net.Listener, error) {
	if host == "" {
		host = "localhost"
	}
	addr := net.JoinHostPort(host, strconv.Itoa(preferred))
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if attempts <= 0 || !isAddrInUse(err) {
		return nil, err
	}
	for i := 1; i <= attempts; i++ {
		addr := net.JoinHostPort(host, strconv.Itoa(preferred+i))
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
		if !isAddrInUse(err) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("no free port in range %d..%d", preferred, preferred+attempts)
}

func isAddrInUse(err error) bool {
	return errors.Is(err, syscall.EADDRINUSE)
}
