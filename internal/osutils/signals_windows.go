//go:build windows

package osutils

import (
	"os"
)

// ShutdownSignals returns the OS signals that should trigger a graceful shutdown
// Windows doesn't formally support SIGTERM gracefully
var ShutdownSignals = []os.Signal{os.Interrupt}
