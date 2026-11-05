//go:build darwin && !cgo

package player

import "errors"

const vtBackendAvailable = false

func newVTSurface(uintptr) (VideoSurface, error) {
	return nil, errors.New("VideoToolbox 测试后端需要启用 cgo")
}

func newVTPlayer(VideoSurface) (*Player, error) {
	return nil, errors.New("VideoToolbox 测试后端需要启用 cgo")
}
