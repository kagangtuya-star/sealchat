//go:build !unix && !windows

package utils

import "errors"

func defaultStartupLockProcessExists(pid int) (bool, error) {
	return false, errors.New("startup lock process check is unsupported on this platform")
}
