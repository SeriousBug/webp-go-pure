//go:build testbenchmark && darwin

package main

import "syscall"

// Darwin reports ru_maxrss in bytes.
func maxRSSBytes() (uint64, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return uint64(ru.Maxrss), nil
}
