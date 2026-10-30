//go:build windows && (amd64 || arm64)

package player

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	videoUser32        = syscall.NewLazyDLL("user32.dll")
	videoWindowVisible = videoUser32.NewProc("IsWindowVisible")
	videoWindowIconic  = videoUser32.NewProc("IsIconic")
	videoWindowDPI     = videoUser32.NewProc("GetDpiForWindow")
	videoFindWindowEx  = videoUser32.NewProc("FindWindowExW")
	videoShowWindow    = videoUser32.NewProc("ShowWindow")
)

const (
	maxRenderWidth  = 1920
	maxRenderHeight = 1080
	maxFrameBuffers = 4
)

// Windows 通过 libmpv Render API 的软件输出绘制到内存，再由 MyGo 作为位图合成，
// 避免 mpv 子窗口被 DirectComposition 层遮挡。
type Surface struct {
	window           uintptr
	visible          atomic.Bool
	dirty            atomic.Bool
	width, height    atomic.Int64
	resizes          atomic.Uint64
	maxResizeNS      atomic.Int64
	callbacks        atomic.Uint64
	rendered         atomic.Uint64
	lastErr          atomic.Pointer[string]
	wake             chan struct{}
	onFrame          func()
	lastW, lastH     int32
	mu               sync.Mutex
	free             [][]byte
	latest           []byte
	latestW, latestH int
	latestStride     int
	buffers          int
}

func NewSurface(window uintptr) (*Surface, error) {
	if window == 0 {
		return nil, fmt.Errorf("无法创建 Windows 视频容器：窗口句柄无效")
	}
	return &Surface{window: window, wake: make(chan struct{}, 1)}, nil
}

func (s *Surface) scale() float64 {
	if videoWindowDPI.Find() == nil {
		if dpi, _, _ := videoWindowDPI.Call(s.window); dpi > 0 {
			return float64(dpi) / 96
		}
	}
	return 1
}

// SetFrameCallback 在每帧写入缓冲后调用；回调需可跨协程触发重绘。
func (s *Surface) SetFrameCallback(cb func()) { s.onFrame = cb }

func (s *Surface) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// SetBounds 将界面尺寸换算成渲染尺寸，按源分辨率上限缩放，避免超采样浪费 CPU。
func (s *Surface) SetBounds(_ float64, _ float64, w, h, _ float64) bool {
	scale := s.scale()
	pw, ph := w*scale, h*scale
	if pw < 1 || ph < 1 {
		return true
	}
	if f := math.Min(1, math.Min(maxRenderWidth/pw, maxRenderHeight/ph)); f < 1 {
		pw, ph = pw*f, ph*f
	}
	tw := max(16, int32(math.Round(pw)))
	th := max(16, int32(math.Round(ph)))
	if tw == s.lastW && th == s.lastH {
		return true
	}
	started := time.Now()
	s.lastW, s.lastH = tw, th
	s.width.Store(int64(tw))
	s.height.Store(int64(th))
	s.dirty.Store(true)
	s.resizes.Add(1)
	ns := time.Since(started).Nanoseconds()
	for old := s.maxResizeNS.Load(); ns > old; old = s.maxResizeNS.Load() {
		if s.maxResizeNS.CompareAndSwap(old, ns) {
			break
		}
	}
	s.notify()
	return true
}

func (s *Surface) Show(show bool) {
	visible, _, _ := videoWindowVisible.Call(s.window)
	iconic, _, _ := videoWindowIconic.Call(s.window)
	show = show && visible != 0 && iconic == 0
	if s.visible.Swap(show) != show {
		s.dirty.Store(true)
		s.notify()
	}
}

// MyGo 在隐藏标题栏时用 MyGoCaption 子窗口绘制系统按钮；它没有公开运行时开关，
// 小窗模式下直接隐藏这些子窗口。
func (s *Surface) SetWindowButtonsVisible(visible bool) {
	if s.window == 0 || videoFindWindowEx.Find() != nil || videoShowWindow.Find() != nil {
		return
	}
	class, err := syscall.UTF16PtrFromString("MyGoCaption")
	if err != nil {
		return
	}
	show := uintptr(0) // SW_HIDE
	if visible {
		show = 8 // SW_SHOWNA
	}
	var after uintptr
	for {
		child, _, _ := videoFindWindowEx.Call(s.window, after, uintptr(unsafe.Pointer(class)), 0)
		if child == 0 {
			break
		}
		videoShowWindow.Call(child, show)
		after = child
	}
}
func (s *Surface) SetCorners(float64, bool) {}

func (s *Surface) acquire(w, h int) ([]byte, int, bool) {
	stride := w * 4
	need := stride * h
	s.mu.Lock()
	defer s.mu.Unlock()
	// 窗口启动和适配视频时会连续改变尺寸。旧尺寸缓冲放不进新帧，
	// 必须释放配额，否则池子被填满后会永远无法分配。
	kept := s.free[:0]
	for _, buf := range s.free {
		if cap(buf) >= need {
			kept = append(kept, buf)
		} else {
			s.buffers--
		}
	}
	s.free = kept
	if n := len(s.free); n > 0 {
		buf := s.free[n-1]
		s.free = s.free[:n-1]
		return buf[:need], stride, true
	}
	if s.buffers >= maxFrameBuffers {
		return nil, 0, false
	}
	s.buffers++
	return make([]byte, need), stride, true
}

// TakeFrame 取出最新一帧的所有权；调用方需在绘制后调用 ReleaseFrame。
func (s *Surface) TakeFrame() ([]byte, int, int, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		return nil, 0, 0, 0, false
	}
	pix, w, h, stride := s.latest, s.latestW, s.latestH, s.latestStride
	s.latest = nil
	return pix, w, h, stride, true
}

func (s *Surface) ReleaseFrame(pix []byte) {
	if pix == nil {
		return
	}
	s.mu.Lock()
	s.free = append(s.free, pix)
	s.mu.Unlock()
	s.notify()
}

func (s *Surface) releaseBuffer(pix []byte) {
	s.ReleaseFrame(pix)
}

func (s *Surface) publish(pix []byte, w, h, stride int) {
	s.mu.Lock()
	if s.latest != nil {
		s.free = append(s.free, s.latest)
	}
	s.latest, s.latestW, s.latestH, s.latestStride = pix, w, h, stride
	s.mu.Unlock()
	s.rendered.Add(1)
	if s.onFrame != nil {
		s.onFrame()
	}
}

func (s *Surface) setRenderError(message string) {
	s.lastErr.Store(&message)
}

func (s *Surface) RenderError() string {
	if message := s.lastErr.Load(); message != nil {
		return *message
	}
	return ""
}

func (s *Surface) Metrics() RenderMetrics {
	return RenderMetrics{
		Resizes:         s.resizes.Load(),
		MaxResizeMS:     float64(s.maxResizeNS.Load()) / 1e6,
		RenderedFrames:  s.rendered.Load(),
		UpdateCallbacks: s.callbacks.Load(),
		Backend:         "libmpv-render-sw",
	}
}
