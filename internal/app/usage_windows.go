//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

func processUsage() (user, system float64, rss int64) {
	var created, ended, kernel, users syscall.Filetime
	process := ^uintptr(0)
	getTimes := syscall.NewLazyDLL("kernel32.dll").NewProc("GetProcessTimes")
	if ok, _, _ := getTimes.Call(process, uintptr(unsafe.Pointer(&created)), uintptr(unsafe.Pointer(&ended)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&users))); ok != 0 {
		user = float64(uint64(users.HighDateTime)<<32|uint64(users.LowDateTime)) / 1e7
		system = float64(uint64(kernel.HighDateTime)<<32|uint64(kernel.LowDateTime)) / 1e7
	}
	var memory struct {
		Size, Faults                                                                           uint32
		PeakWorking, Working, PeakPaged, Paged, PeakNonPaged, NonPaged, Pagefile, PeakPagefile uintptr
	}
	memory.Size = uint32(unsafe.Sizeof(memory))
	getMemory := syscall.NewLazyDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")
	if ok, _, _ := getMemory.Call(process, uintptr(unsafe.Pointer(&memory)), uintptr(memory.Size)); ok != 0 {
		rss = int64(memory.PeakWorking)
	}
	return
}
