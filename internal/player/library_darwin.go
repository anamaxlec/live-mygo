//go:build darwin

package player

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/ebitengine/purego"
)

func loadLibrary() (uintptr, error) {
	paths := []string{os.Getenv("LIVE_MYGO_LIBMPV")}
	if paths[0] == "" {
		if executable, err := os.Executable(); err == nil {
			paths = append(paths, filepath.Join(filepath.Dir(executable), "../Frameworks/libmpv.2.dylib"))
		}
		paths = append(paths, "/opt/homebrew/opt/mpv/lib/libmpv.dylib", "/usr/local/opt/mpv/lib/libmpv.dylib", "libmpv.dylib")
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		if lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
			return lib, nil
		}
	}
	return 0, errors.New("找不到 libmpv，请安装 mpv 或设置 LIVE_MYGO_LIBMPV")
}

func librarySymbol(lib uintptr, name string) (uintptr, error) { return purego.Dlsym(lib, name) }

func surfaceOptions(*Surface) [][2]string {
	return [][2]string{{"vo", "libmpv"}, {"video-timing-offset", "0"}}
}
