//go:build !darwin

package surface

import "os"

// Other platforms keep the CLI noninteractive.
func InteractiveTerminal(stdin, stderr *os.File) bool { return false }
