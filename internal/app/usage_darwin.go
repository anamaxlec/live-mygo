//go:build darwin

package app

import "syscall"

func processUsage() (user, system float64, rss int64) {
	var usage syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &usage)
	return float64(usage.Utime.Sec) + float64(usage.Utime.Usec)/1e6, float64(usage.Stime.Sec) + float64(usage.Stime.Usec)/1e6, usage.Maxrss
}
