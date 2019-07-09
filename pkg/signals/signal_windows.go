/*
brutally copy-pasted from the official sample controller project
*/

package signals

import "os"

var shutdownSignals = []os.Signal{os.Interrupt}
