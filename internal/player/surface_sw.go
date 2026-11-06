//go:build darwin || windows

package player

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxRenderWidth  = 1920
	maxRenderHeight = 1080
	maxFrameBuffers = 4
)

// softwareSurface stores libmpv frames for MyGo to composite without a native video view.
type softwareSurface struct {
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

func newSoftwareSurface() *softwareSurface {
	return &softwareSurface{wake: make(chan struct{}, 1)}
}

func (s *softwareSurface) renderVisible() bool        { return s.visible.Load() }
func (s *softwareSurface) renderSize() (int64, int64) { return s.width.Load(), s.height.Load() }
func (s *softwareSurface) mpvOptions() [][2]string    { return [][2]string{{"vo", "libmpv"}} }

// SetFrameCallback 在每帧写入缓冲后调用；回调需可跨协程触发重绘。
func (s *softwareSurface) SetFrameCallback(cb func()) { s.onFrame = cb }

func (s *softwareSurface) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// SetBounds 将界面尺寸换算成渲染尺寸，按源分辨率上限缩放，避免超采样浪费 CPU。
func (s *softwareSurface) SetBounds(_ float64, _ float64, w, h, _ float64) bool {
	return s.resize(w, h)
}

func (s *softwareSurface) resize(pw, ph float64) bool {
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

func (s *softwareSurface) Show(show bool) {
	if s.visible.Swap(show) != show {
		s.dirty.Store(true)
		s.notify()
	}
}

func (s *softwareSurface) SetCorners(float64, bool)     {}
func (s *softwareSurface) SetWindowButtonsVisible(bool) {}

func (s *softwareSurface) acquire(w, h int) ([]byte, int, bool) {
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
func (s *softwareSurface) TakeFrame() ([]byte, int, int, int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.latest == nil {
		return nil, 0, 0, 0, false
	}
	pix, w, h, stride := s.latest, s.latestW, s.latestH, s.latestStride
	s.latest = nil
	return pix, w, h, stride, true
}

func (s *softwareSurface) ReleaseFrame(pix []byte) {
	if pix == nil {
		return
	}
	s.mu.Lock()
	s.free = append(s.free, pix)
	s.mu.Unlock()
	s.notify()
}

func (s *softwareSurface) releaseBuffer(pix []byte) {
	s.ReleaseFrame(pix)
}

func (s *softwareSurface) publish(pix []byte, w, h, stride int) {
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

func (s *softwareSurface) setRenderError(message string) {
	s.lastErr.Store(&message)
}

func (s *softwareSurface) RenderError() string {
	if message := s.lastErr.Load(); message != nil {
		return *message
	}
	return ""
}

func (s *softwareSurface) Metrics() RenderMetrics {
	return RenderMetrics{
		Resizes:         s.resizes.Load(),
		MaxResizeMS:     float64(s.maxResizeNS.Load()) / 1e6,
		RenderedFrames:  s.rendered.Load(),
		UpdateCallbacks: s.callbacks.Load(),
		Backend:         "libmpv-render-sw",
	}
}

func (s *softwareSurface) Close() {}
