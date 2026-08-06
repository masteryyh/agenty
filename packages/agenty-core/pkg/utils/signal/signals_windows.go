package signal

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{syscall.SIGINT}
