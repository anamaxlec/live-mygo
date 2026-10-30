//go:build windows && (amd64 || arm64)

package player

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func loadLibrary() (uintptr, error) {
	paths := []string{os.Getenv("LIVE_MYGO_LIBMPV")}
	if paths[0] == "" {
		executable, err := os.Executable()
		if err != nil {
			return 0, err
		}
		paths = []string{filepath.Join(filepath.Dir(executable), "libmpv-2.dll"), filepath.Join(filepath.Dir(executable), "mpv-2.dll")}
	}
	load := syscall.NewLazyDLL("kernel32.dll").NewProc("LoadLibraryExW")
	var last error
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			last = err
			continue
		}
		name, err := syscall.UTF16PtrFromString(absolute)
		if err != nil {
			last = err
			continue
		}
		// 依赖从 DLL 所在目录和系统目录加载，不搜索当前工作目录。
		lib, _, err := load.Call(uintptr(unsafe.Pointer(name)), 0, 0x1100)
		if lib != 0 {
			return lib, nil
		}
		last = err
	}
	return 0, fmt.Errorf("无法加载 Windows libmpv，请将 libmpv-2.dll 放在程序旁边：%w", last)
}

func librarySymbol(lib uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(lib), name)
}

func surfaceOptions(*Surface) [][2]string {
	return [][2]string{{"vo", "libmpv"}}
}
