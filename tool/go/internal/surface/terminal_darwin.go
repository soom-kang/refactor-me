//go:build darwin

package surface

import (
	"os"
	"syscall"
	"unsafe"
)

// InteractiveTerminal requires terminal input and a terminal for the prompt.
func InteractiveTerminal(stdin, stderr *os.File) bool {
	return terminalFile(stdin) && terminalFile(stderr)
}

func terminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	var termios syscall.Termios
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&termios)))
	return err == 0
}
