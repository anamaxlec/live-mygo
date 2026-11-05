//go:build darwin || windows

package player

import "os"

// 默认保留 mpv；原生内核由用户在播放设置中选择。
func DefaultBackend() string       { return "mpv" }
func NativeBackendAvailable() bool { return vtBackendAvailable }
func NormalizeBackend(name string) string {
	if name == "vt" && vtBackendAvailable {
		return "vt"
	}
	return "mpv"
}
func selectedBackend(choices []string) string {
	name := DefaultBackend()
	if len(choices) > 0 {
		name = choices[0]
	}
	if env := os.Getenv("LIVE_MYGO_PLAYER"); env != "" {
		name = env
	}
	return NormalizeBackend(name)
}
