//go:build darwin

package app

// macOS 由原生 GL 视图呈现视频，不需要从播放器搬运帧。
func (a *app) takeVideoFrame() {}
