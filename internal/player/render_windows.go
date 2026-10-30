//go:build windows && (amd64 || arm64)

package player

import (
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type renderParam struct {
	Type int32
	_    int32
	Data unsafe.Pointer
}

type renderer struct {
	stop    chan struct{}
	done    chan struct{}
	surface *Surface
}

func (r *renderer) close() {
	close(r.stop)
	<-r.done
}

// initRenderer 在固定线程上创建软件渲染上下文。mpv 按视频时钟阻塞每次 render，
// 从而保持音画同步；轮询新帧状态，避免从 mpv 线程回调 Go。
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
		var free func(uintptr)
		for _, binding := range []struct {
			name string
			fn   any
		}{
			{"mpv_render_context_create", &create},
			{"mpv_render_context_render", &render},
			{"mpv_render_context_update", &update},
			{"mpv_render_context_free", &free},
		} {
			addr, err := librarySymbol(lib, binding.name)
			if err != nil {
				ready <- fmt.Errorf("libmpv 缺少 %s", binding.name)
				return
			}
			purego.RegisterFunc(binding.fn, addr)
		}
		api := []byte("sw\x00")
		params := []renderParam{{Type: 1, Data: unsafe.Pointer(&api[0])}, {}}
		var ctx uintptr
		if rc := create(&ctx, p.handle, &params[0]); rc < 0 {
			runtime.KeepAlive(api)
			ready <- fmt.Errorf("libmpv Render API 初始化失败：%s", p.api.errorString(rc))
			return
		}
		defer free(ctx)
		ready <- nil
		ticker := time.NewTicker(4 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.stop:
				return
			case <-ticker.C:
			}
			if !s.visible.Load() {
				continue
			}
			flags := update(ctx)
			if flags&1 != 0 {
				s.callbacks.Add(1)
			}
			dirty := s.dirty.Swap(false)
			if flags&1 == 0 && !dirty {
				continue
			}
			w, h := int(s.width.Load()), int(s.height.Load())
			if w <= 0 || h <= 0 {
				continue
			}
			buf, stride, ok := s.acquire(w, h)
			if !ok {
				s.dirty.Store(true)
				continue
			}
			size := [2]int32{int32(w), int32(h)}
			format := []byte("rgb0\x00")
			swStride := uintptr(stride)
			renderParams := []renderParam{
				{Type: 17, Data: unsafe.Pointer(&size[0])},
				{Type: 18, Data: unsafe.Pointer(&format[0])},
				{Type: 19, Data: unsafe.Pointer(&swStride)},
				{Type: 20, Data: unsafe.Pointer(&buf[0])},
				{},
			}
			rc := render(ctx, &renderParams[0])
			runtime.KeepAlive(buf)
			runtime.KeepAlive(format)
			if rc < 0 {
				s.setRenderError("libmpv 视频呈现失败：" + p.api.errorString(rc))
				s.releaseBuffer(buf)
				continue
			}
			// rgb0 的第四字节未定义，MyGo 按不透明像素合成。
			for i := 3; i < len(buf); i += 4 {
				buf[i] = 255
			}
			s.publish(buf, w, h, stride)
		}
	}()
	if err := <-ready; err != nil {
		<-r.done
		return err
	}
	p.renderer = r
	return nil
}
