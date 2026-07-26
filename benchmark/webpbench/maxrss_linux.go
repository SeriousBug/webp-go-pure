//go:build testbenchmark && linux

package main

import "syscall"

// Linux reports ru_maxrss in kilobytes.
func maxRSSBytes() (uint64, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, err
	}
	return uint64(ru.Maxrss) * 1024, nil
}
