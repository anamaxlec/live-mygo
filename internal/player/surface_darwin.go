//go:build darwin

package player

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

type point struct{ X, Y float64 }
type size struct{ W, H float64 }
type rect struct {
	Origin point
	Size   size
}

// MyGo 继续绘制原生 UI；视频通过 libmpv Render API 绘制到独立的原生 GL 视图。
type Surface struct {
	view, parent, window objc.ID
	context              uintptr
	nativeContext        objc.ID
	glLib                uintptr
	setRect              func(objc.ID, objc.SEL, rect)
	getFloat             func(objc.ID, objc.SEL) float64
	cornerViews          [4]objc.ID
	cornerImages         [4]objc.ID
	setImageSize         func(objc.ID, objc.SEL, size)
	cornerRadius         float64
	cornerRight          bool
	makeCurrent          func(uintptr) int32
	flush                func(uintptr) int32
	lock, unlock         func(uintptr) int32
	last                 rect
	width, height        atomic.Int64
	visible              atomic.Bool
	dirty                atomic.Bool
	resizePending        atomic.Bool
	frameMu              sync.Mutex
	wake                 chan struct{}
	resizes, deferred    atomic.Uint64
	rendered, callbacks  atomic.Uint64
	maxResizeNS          atomic.Int64
}

func NewSurface(window uintptr) (*Surface, error) {
	lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	gl, err := purego.Dlopen("/System/Library/Frameworks/OpenGL.framework/OpenGL", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, err
	}
	sym, err := purego.Dlsym(lib, "objc_msgSend")
	if err != nil {
		return nil, err
	}
	s := &Surface{window: objc.ID(window), parent: objc.ID(window).Send(objc.RegisterName("contentView")), glLib: gl, wake: make(chan struct{}, 1)}
	if s.parent == 0 {
		return nil, errors.New("MyGo 窗口没有原生 contentView")
	}
	purego.RegisterFunc(&s.setRect, sym)
	purego.RegisterFunc(&s.getFloat, sym)
	for _, binding := range []struct {
		name string
		fn   any
	}{{"CGLSetCurrentContext", &s.makeCurrent}, {"CGLFlushDrawable", &s.flush}, {"CGLLockContext", &s.lock}, {"CGLUnlockContext", &s.unlock}} {
		addr, err := purego.Dlsym(gl, binding.name)
		if err != nil {
			return nil, err
		}
		purego.RegisterFunc(binding.fn, addr)
	}
	attrs := []uint32{99, 0x3200, 5, 73, 8, 24, 11, 8, 0}
	format := objc.ID(objc.GetClass("NSOpenGLPixelFormat")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithAttributes:"), uintptr(unsafe.Pointer(&attrs[0])))
	runtime.KeepAlive(attrs)
	if format == 0 {
		return nil, errors.New("无法创建原生 OpenGL 像素格式")
	}
	var initView func(objc.ID, objc.SEL, rect, objc.ID) objc.ID
	purego.RegisterFunc(&initView, sym)
	s.view = initView(objc.ID(objc.GetClass("NSOpenGLView")).Send(objc.RegisterName("alloc")), objc.RegisterName("initWithFrame:pixelFormat:"), rect{Size: size{640, 360}}, format)
	format.Send(objc.RegisterName("release"))
	if s.view == 0 {
		return nil, errors.New("无法创建原生视频视图")
	}
	s.view.Send(objc.RegisterName("setWantsBestResolutionOpenGLSurface:"), true)
	s.view.Send(objc.RegisterName("setHidden:"), true)
	// 与 MyGo 的 WebView 一样，将原生画面放在 GPU UI 之下，控件可直接叠加。
	subviews := s.parent.Send(objc.RegisterName("subviews"))
	var canvas objc.ID
	for i := uintptr(0); i < uintptr(subviews.Send(objc.RegisterName("count"))); i++ {
		view := subviews.Send(objc.RegisterName("objectAtIndex:"), i)
		if view.Send(objc.RegisterName("isKindOfClass:"), objc.ID(objc.GetClass("MyGoSurfaceView"))) != 0 {
			canvas = view
			break
		}
	}
	if canvas == 0 {
		return nil, errors.New("找不到 MyGo 原生绘制视图")
	}
	s.parent.Send(objc.RegisterName("addSubview:positioned:relativeTo:"), s.view, ^uintptr(0), canvas)
	context := s.view.Send(objc.RegisterName("openGLContext"))
	s.nativeContext = context
	s.context = uintptr(context.Send(objc.RegisterName("CGLContextObj")))
	if s.context == 0 {
		return nil, errors.New("无法创建原生 OpenGL context")
	}
	interval := int32(1)
	context.Send(objc.RegisterName("setValues:forParameter:"), uintptr(unsafe.Pointer(&interval)), 222)
	runtime.KeepAlive(interval)
	if err := s.initMaterialCorners(sym, canvas); err != nil {
		return nil, err
	}
	s.width.Store(1280)
	s.height.Store(720)
	return s, nil
}

// 与窗口布局一起在 UI 线程调用；小窗使用画面内的浮动标题操作。
func (s *Surface) SetWindowButtonsVisible(visible bool) {
	for _, kind := range []uintptr{0, 1, 2} {
		button := s.window.Send(objc.RegisterName("standardWindowButton:"), kind)
		if button != 0 {
			button.Send(objc.RegisterName("setHidden:"), !visible)
		}
	}
}

// 渲染线程可能等待下一次垂直同步，主线程不在此等待它释放 drawable。
// 本帧忙时交由下一次 UI 帧重试，只合并到最新尺寸。
func (s *Surface) SetBounds(x, y, w, h, windowHeight float64) bool {
	if s.parent.Send(objc.RegisterName("isFlipped")) == 0 {
		y = windowHeight - y - h
	}
	r := rect{point{x, y}, size{w, h}}
	scale := s.getFloat(s.window, objc.RegisterName("backingScaleFactor"))
	if scale <= 0 {
		scale = 1
	}
	pw, ph := int64(w*scale), int64(h*scale)
	if r != s.last || pw != s.width.Load() || ph != s.height.Load() {
		s.resizePending.Store(true)
		if !s.frameMu.TryLock() {
			s.deferred.Add(1)
			s.notify()
			return false
		}
		defer s.frameMu.Unlock()
		started := time.Now()
		s.lock(s.context)
		s.setRect(s.view, objc.RegisterName("setFrame:"), r)
		s.nativeContext.Send(objc.RegisterName("update"))
		s.unlock(s.context)
		s.last = r
		s.updateMaterialCorners()
		s.width.Store(pw)
		s.height.Store(ph)
		s.dirty.Store(true)
		s.resizePending.Store(false)
		s.resizes.Add(1)
		ns := time.Since(started).Nanoseconds()
		for old := s.maxResizeNS.Load(); ns > old; old = s.maxResizeNS.Load() {
			if s.maxResizeNS.CompareAndSwap(old, ns) {
				break
			}
		}
		s.notify()
	} else if s.resizePending.Swap(false) {
		s.notify()
	}
	return true
}

func (s *Surface) Show(show bool) {
	// 完全被遮挡或最小化时停止提交视频帧，恢复可见后由下一次 UI / 状态更新唤醒。
	// AppKit 的窗口状态只在主线程读取。
	show = show && s.window.Send(objc.RegisterName("isVisible")) != 0 && s.window.Send(objc.RegisterName("occlusionState"))&(1<<1) != 0
	if s.visible.Swap(show) != show {
		s.view.Send(objc.RegisterName("setHidden:"), !show)
		s.updateMaterialCorners()
		s.dirty.Store(true)
		s.notify()
	}
}

func (s *Surface) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Surface) Metrics() RenderMetrics {
	return RenderMetrics{Resizes: s.resizes.Load(), DeferredResizes: s.deferred.Load(), MaxResizeMS: float64(s.maxResizeNS.Load()) / 1e6, RenderedFrames: s.rendered.Load(), UpdateCallbacks: s.callbacks.Load(), Backend: "libmpv-render-opengl"}
}

func (s *Surface) RenderError() string { return "" }

// Windows 的软件输出用回调驱动重绘；macOS 的原生视图自行提交帧。
func (s *Surface) SetFrameCallback(func()) {}
