//go:build darwin

package player

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

type renderParam struct {
	Type int32
	_    int32
	Data unsafe.Pointer
}
type glInit struct {
	GetProc uintptr
	Context uintptr
}
type glFBO struct{ ID, Width, Height, Format int32 }
type renderer struct {
	stop    chan struct{}
	done    chan struct{}
	surface *Surface
}

func (r *renderer) close() {
	close(r.stop)
	<-r.done
}

func (p *Player) initRenderer(lib uintptr, s *Surface) error {
	r := &renderer{stop: make(chan struct{}), done: make(chan struct{}), surface: s}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(r.done)
		var create func(*uintptr, uintptr, *renderParam) int32
		var render func(uintptr, *renderParam) int32
		var update func(uintptr) uint64
		var setUpdate func(uintptr, uintptr, uintptr)
		var free func(uintptr)
		for _, b := range []struct {
			name string
			fn   any
		}{{"mpv_render_context_create", &create}, {"mpv_render_context_render", &render}, {"mpv_render_context_update", &update}, {"mpv_render_context_set_update_callback", &setUpdate}, {"mpv_render_context_free", &free}} {
			addr, err := purego.Dlsym(lib, b.name)
			if err != nil {
				ready <- err
				return
			}
			purego.RegisterFunc(b.fn, addr)
		}
		if rc := s.makeCurrent(s.context); rc != 0 {
			ready <- fmt.Errorf("CGL context 初始化失败：%d", rc)
			return
		}
		defer s.makeCurrent(0)
		proc := purego.NewCallback(func(_ uintptr, name unsafe.Pointer) uintptr {
			ptr, _ := purego.Dlsym(s.glLib, cstring(name))
			return ptr
		})
		gl := glInit{GetProc: proc}
		api := []byte("opengl\x00")
		params := []renderParam{{Type: 1, Data: unsafe.Pointer(&api[0])}, {Type: 2, Data: unsafe.Pointer(&gl)}, {}}
		var ctx uintptr
		if rc := create(&ctx, p.handle, &params[0]); rc < 0 {
			ready <- fmt.Errorf("libmpv Render API 初始化失败：%s", p.api.errorString(rc))
			return
		}
		defer func() { s.frameMu.Lock(); s.lock(s.context); free(ctx); s.unlock(s.context); s.frameMu.Unlock() }()
		callback := purego.NewCallback(func(_ uintptr) { s.callbacks.Add(1); s.notify() })
		setUpdate(ctx, callback, 0)
		defer setUpdate(ctx, 0, 0)
		ready <- nil
		for {
			select {
			case <-r.stop:
				return
			case <-s.wake:
				if !s.visible.Load() {
					continue
				}
				// 主线程的最新尺寸优先于下一帧，避免连续播放时尺寸更新饥饿。
				if s.resizePending.Load() {
					continue
				}
				changed := s.dirty.Swap(false)
				if update(ctx)&1 == 0 && !changed {
					continue
				}
				s.frameMu.Lock()
				fbo := glFBO{Width: int32(s.width.Load()), Height: int32(s.height.Load())}
				if fbo.Width <= 0 || fbo.Height <= 0 {
					s.frameMu.Unlock()
					continue
				}
				flip := int32(1)
				params := []renderParam{{Type: 3, Data: unsafe.Pointer(&fbo)}, {Type: 4, Data: unsafe.Pointer(&flip)}, {}}
				s.lock(s.context)
				render(ctx, &params[0])
				s.flush(s.context)
				s.unlock(s.context)
				s.frameMu.Unlock()
				s.rendered.Add(1)
			}
		}
	}()
	if err := <-ready; err != nil {
		<-r.done
		return err
	}
	p.renderer = r
	return nil
}
