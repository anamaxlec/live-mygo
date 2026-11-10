//go:build darwin

package player

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

type glInit struct {
	GetProc uintptr
	Context uintptr
}
type glFBO struct{ ID, Width, Height, Format int32 }

func (s *Surface) startRenderer(p *Player, lib uintptr) error {
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
		// VideoToolbox 帧不能由 swscale 直接读取；截图交给 GPU 渲染线程处理。
		// advanced control 要求即使窗口隐藏也处理 update，下面只跳过实际呈现。
		advanced := int32(1)
		params := []renderParam{{Type: 1, Data: unsafe.Pointer(&api[0])}, {Type: 2, Data: unsafe.Pointer(&gl)}, {Type: 10, Data: unsafe.Pointer(&advanced)}, {}}
		var ctx uintptr
		s.frameMu.Lock()
		s.lock(s.context)
		rc := create(&ctx, p.handle, &params[0])
		s.unlock(s.context)
		s.frameMu.Unlock()
		if rc < 0 {
			ready <- fmt.Errorf("libmpv Render API 初始化失败：%s", p.api.errorString(rc))
			return
		}
		var picture glPictureTarget
		defer func() {
			s.frameMu.Lock()
			s.lock(s.context)
			picture.close()
			free(ctx)
			s.unlock(s.context)
			s.frameMu.Unlock()
		}()
		callback := purego.NewCallback(func(_ uintptr) { s.callbacks.Add(1); s.notify() })
		setUpdate(ctx, callback, 0)
		defer setUpdate(ctx, 0, 0)
		ready <- nil
		for {
			select {
			case <-r.stop:
				return
			case <-s.wake:
				s.frameMu.Lock()
				s.lock(s.context)
				if rc := s.makeCurrent(s.context); rc != 0 {
					message := fmt.Sprintf("视频 context 恢复失败：%d", rc)
					s.renderErr.Store(&message)
					s.unlock(s.context)
					s.frameMu.Unlock()
					continue
				}
				flags := update(ctx)
				pictureRendered := flags&1 != 0 && picture.draw(s, ctx, render)
				// update 也会处理截图等 GPU 请求，不能因遮挡或调整尺寸略过。
				if !s.visible.Load() || s.resizePending.Load() {
					if flags&1 != 0 && !pictureRendered {
						skip := int32(1)
						params := []renderParam{{Type: 13, Data: unsafe.Pointer(&skip)}, {}}
						render(ctx, &params[0])
					}
					s.unlock(s.context)
					s.frameMu.Unlock()
					continue
				}
				changed := s.dirty.Swap(false)
				if flags&1 == 0 && !changed {
					s.unlock(s.context)
					s.frameMu.Unlock()
					continue
				}
				fbo := glFBO{Width: int32(s.width.Load()), Height: int32(s.height.Load())}
				if fbo.Width <= 0 || fbo.Height <= 0 {
					s.unlock(s.context)
					s.frameMu.Unlock()
					continue
				}
				flip := int32(1)
				params := []renderParam{{Type: 3, Data: unsafe.Pointer(&fbo)}, {Type: 4, Data: unsafe.Pointer(&flip)}, {}}
				if rc := render(ctx, &params[0]); rc < 0 {
					message := fmt.Sprintf("视频帧渲染失败：%d", rc)
					s.renderErr.Store(&message)
				}
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
