//go:build !windows

package osutils

import (
	"os"
	"syscall"
)

// ShutdownSignals returns the OS signals that should trigger a graceful shutdown
var ShutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
