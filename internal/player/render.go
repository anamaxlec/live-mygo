//go:build darwin || windows

package player

import "unsafe"

type mpvSurface interface {
	VideoSurface
	mpvOptions() [][2]string
	startRenderer(*Player, uintptr) error
	renderVisible() bool
	renderSize() (int64, int64)
}

type renderParam struct {
	Type int32
	_    int32
	Data unsafe.Pointer
}

type renderer struct {
	stop    chan struct{}
	done    chan struct{}
	surface mpvSurface
}

func (r *renderer) close() {
	close(r.stop)
	<-r.done
}
