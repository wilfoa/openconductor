//go:build darwin

package attention

import "syscall"

// CheckProcess determines the state of a process identified by pid on macOS.
//
// It uses syscall.Kill with signal 0 to probe whether the process is alive
// without actually sending a signal. If the process exists, Running is
// returned; otherwise Exited is returned.
func CheckProcess(pid int) ProcessState {
	if pid <= 0 {
		return Unknown
	}

	err := syscall.Kill(pid, 0)
	if err == nil {
		return Running
	}

	return Exited
}
