package main

import (
	"os"
	"os/signal"
	"syscall"
)

// registerShutdownSignal creates a buffered channel and subscribes it to
// SIGINT and SIGTERM. Callers receive on the returned channel inside
// their own select to coordinate graceful shutdown. The buffer size is 1
// so signal.Notify can deliver without blocking if the receiver hasn't
// selected yet.
func registerShutdownSignal() chan os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	return sigCh
}
