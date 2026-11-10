//go:build darwin && cgo

package player

/*
#cgo LDFLAGS: -framework AVKit -framework OpenGL
#include "picture_in_picture_darwin.h"
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type PictureInPictureEvent int

const (
	PictureInPictureStopped PictureInPictureEvent = iota + 1
	PictureInPictureFailed
	PictureInPicturePlay
	PictureInPicturePause
)

type pictureInPictureSlot struct {
	picture atomic.Pointer[PictureInPicture]
}

type PictureInPicture struct {
	slot     *pictureInPictureSlot
	mu       sync.Mutex
	native   unsafe.Pointer
	callback cgo.Handle
}

// StartPictureInPicture connects an existing video output to the macOS player.
// It must be called on the main thread, like Close and SetPaused.
func StartPictureInPicture(surface VideoSurface, window uintptr, paused bool, onEvent func(PictureInPictureEvent)) (*PictureInPicture, error) {
	var slot *pictureInPictureSlot
	var view uintptr
	switch s := surface.(type) {
	case *VTSurface:
		slot, view = &s.pictureInPictureSlot, uintptr(s.view)
	case *Surface:
		slot, view = &s.pictureInPictureSlot, uintptr(s.view)
	default:
		return nil, errors.New("当前视频输出不支持系统画中画")
	}
	if slot.picture.Load() != nil {
		return nil, errors.New("系统画中画已打开")
	}
	callback := cgo.NewHandle(onEvent)
	native := C.lm_pip_create(C.uintptr_t(window), C.uintptr_t(view), C.uintptr_t(callback), boolInt(paused))
	if native == nil {
		callback.Delete()
		return nil, errors.New("此设备无法开启系统画中画")
	}
	p := &PictureInPicture{slot: slot, native: native, callback: callback}
	slot.picture.Store(p)
	return p, nil
}

//export liveMyGoPictureInPictureEvent
func liveMyGoPictureInPictureEvent(handle C.uintptr_t, event C.int) {
	cgo.Handle(handle).Value().(func(PictureInPictureEvent))(PictureInPictureEvent(event))
}

func (p *PictureInPicture) Close() {
	p.slot.picture.CompareAndSwap(p, nil)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.native != nil {
		C.lm_pip_close(p.native)
		p.native = nil
		p.callback.Delete()
	}
}

func (p *PictureInPicture) SetPaused(paused bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.native != nil {
		C.lm_pip_pause(p.native, boolInt(paused))
	}
}

func (s *pictureInPictureSlot) pictureInPictureVisible() bool { return s.picture.Load() != nil }

func (s *pictureInPictureSlot) enqueuePicture(frame unsafe.Pointer) bool {
	p := s.picture.Load()
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.native != nil && C.lm_pip_enqueue(p.native, frame) != 0
}

type glPictureTarget struct {
	buffer    unsafe.Pointer
	fbo       glFBO
	lastFrame time.Time
}

func (t *glPictureTarget) close() {
	if t.buffer != nil {
		C.lm_pip_gl_close(t.buffer)
		*t = glPictureTarget{}
	}
}

func (t *glPictureTarget) draw(s *Surface, ctx uintptr, render func(uintptr, *renderParam) int32) bool {
	p := s.picture.Load()
	if p == nil {
		t.close()
		return false
	}
	if time.Since(t.lastFrame) < time.Second/30 {
		return false
	}
	w, h := int32(s.width.Load()), int32(s.height.Load())
	if w < 1 || h < 1 {
		return false
	}
	if longest := max(w, h); longest > 640 {
		w, h = max(1, w*640/longest), max(1, h*640/longest)
	}
	if t.buffer == nil || t.fbo.Width != w || t.fbo.Height != h {
		t.close()
		var fbo C.uint32_t
		t.buffer = C.lm_pip_gl_create(C.int(w), C.int(h), &fbo)
		t.fbo = glFBO{ID: int32(fbo), Width: w, Height: h}
		if t.buffer == nil {
			return false
		}
	}
	// Readback is in framebuffer order, the opposite of the window drawable.
	flip, wait := int32(0), int32(0)
	params := []renderParam{{Type: 3, Data: unsafe.Pointer(&t.fbo)}, {Type: 4, Data: unsafe.Pointer(&flip)}, {Type: 12, Data: unsafe.Pointer(&wait)}, {}}
	if render(ctx, &params[0]) < 0 {
		return false
	}
	t.lastFrame = time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.native != nil {
		C.lm_pip_gl_enqueue(p.native, t.buffer)
	}
	return true
}
