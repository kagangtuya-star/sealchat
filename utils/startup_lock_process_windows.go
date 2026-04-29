//go:build windows

package utils

import (
	"syscall"
)

const startupLockWindowsProcessQueryLimitedInformation = 0x1000
const startupLockWindowsErrInvalidParameter syscall.Errno = 87
const startupLockWindowsErrAccessDenied syscall.Errno = 5

func defaultStartupLockProcessExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := syscall.OpenProcess(startupLockWindowsProcessQueryLimitedInformation, false, uint32(pid))
	if err == nil {
		_ = syscall.CloseHandle(handle)
		return true, nil
	}
	if errno, ok := err.(syscall.Errno); ok {
		switch errno {
		case startupLockWindowsErrInvalidParameter:
			return false, nil
		case startupLockWindowsErrAccessDenied:
			return true, nil
		}
	}
	return false, err
}
