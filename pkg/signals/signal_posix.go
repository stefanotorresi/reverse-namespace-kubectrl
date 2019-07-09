/*
brutally copy-pasted from the official sample controller project
*/

// +build !windows

package signals

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
