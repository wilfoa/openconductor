// SPDX-License-Identifier: MIT
// Copyright (c) 2025 The OpenConductor Authors.

//go:build linux

package attention

import "syscall"

// CheckProcess determines the state of a process identified by pid on Linux.
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
