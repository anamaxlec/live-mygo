//go:build windows

package player

import "errors"

const vtBackendAvailable = false

func newVTPlayer(VideoSurface) (*Player, error) {
	return nil, errors.New("VideoToolbox 后端仅支持 macOS")
}
