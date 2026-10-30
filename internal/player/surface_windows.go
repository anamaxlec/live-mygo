//go:build windows && (amd64 || arm64)

package player

import (
	"fmt"
	"math"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	videoUser32            = syscall.NewLazyDLL("user32.dll")
	videoGDI32             = syscall.NewLazyDLL("gdi32.dll")
	createVideoWindow      = videoUser32.NewProc("CreateWindowExW")
	moveVideoWindow        = videoUser32.NewProc("SetWindowPos")
	showVideoWindow        = videoUser32.NewProc("ShowWindow")
	videoWindowVisible     = videoUser32.NewProc("IsWindowVisible")
	videoWindowIconic      = videoUser32.NewProc("IsIconic")
	videoWindowDPI         = videoUser32.NewProc("GetDpiForWindow")
	findCaptionWindow      = videoUser32.NewProc("FindWindowExW")
	setVideoRegion         = videoUser32.NewProc("SetWindowRgn")
	createVideoRoundRegion = videoGDI32.NewProc("CreateRoundRectRgn")
	createVideoRectRegion  = videoGDI32.NewProc("CreateRectRgn")
	combineVideoRegion     = videoGDI32.NewProc("CombineRgn")
	deleteVideoRegion      = videoGDI32.NewProc("DeleteObject")
)

// Windows 使用 mpv 的原生 D3D11 输出；视频子窗口在 MyGo UI 之下。
// 控件和画面弹幕仍由同一套 MyGo 界面绘制，尺寸更新不等待视频呈现。
type Surface struct {
	view, window, caption      uintptr
	visible                    atomic.Bool
	width, height              atomic.Int64
	resizes                    atomic.Uint64
	maxResizeNS                atomic.Int64
	last                       [4]int32
	radius                     float64
	roundRight, buttonsVisible bool
}

func NewSurface(window uintptr) (*Surface, error) {
	class, _ := syscall.UTF16PtrFromString("Static")
	view, _, err := createVideoWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0x40000000|0x08000000|0x04000000|4, 0, 0, 640, 360, window, 0, 0, 0)
	if view == 0 {
		return nil, fmt.Errorf("无法创建 Windows 视频容器：%w", err)
	}
	s := &Surface{view: view, window: window, buttonsVisible: true}
	caption, _ := syscall.UTF16PtrFromString("MyGoCaption")
	s.caption, _, _ = findCaptionWindow.Call(window, 0, uintptr(unsafe.Pointer(caption)), 0)
	moveVideoWindow.Call(view, 1, 0, 0, 640, 360, 0x0010)
	return s, nil
}

func (s *Surface) scale() float64 {
	if videoWindowDPI.Find() == nil {
		if dpi, _, _ := videoWindowDPI.Call(s.window); dpi > 0 {
			return float64(dpi) / 96
		}
	}
	return 1
}

func (s *Surface) SetBounds(x, y, w, h, _ float64) bool {
	scale := s.scale()
	r := [4]int32{int32(math.Round(x * scale)), int32(math.Round(y * scale)), int32(max(1, math.Round(w*scale))), int32(max(1, math.Round(h*scale)))}
	if r == s.last {
		return true
	}
	started := time.Now()
	if ok, _, _ := moveVideoWindow.Call(s.view, 1, uintptr(r[0]), uintptr(r[1]), uintptr(r[2]), uintptr(r[3]), 0x0010); ok == 0 {
		return false
	}
	s.last = r
	s.width.Store(int64(r[2]))
	s.height.Store(int64(r[3]))
	s.updateRegion()
	s.applyButtons()
	s.resizes.Add(1)
	ns := time.Since(started).Nanoseconds()
	for old := s.maxResizeNS.Load(); ns > old; old = s.maxResizeNS.Load() {
		if s.maxResizeNS.CompareAndSwap(old, ns) {
			break
		}
	}
	return true
}

func (s *Surface) Show(show bool) {
	visible, _, _ := videoWindowVisible.Call(s.window)
	iconic, _, _ := videoWindowIconic.Call(s.window)
	show = show && visible != 0 && iconic == 0
	if s.visible.Swap(show) != show {
		state := uintptr(0)
		if show {
			state = 4 // SW_SHOWNOACTIVATE
		}
		showVideoWindow.Call(s.view, state)
	}
}

func (s *Surface) SetWindowButtonsVisible(visible bool) {
	s.buttonsVisible = visible
	s.applyButtons()
}

func (s *Surface) applyButtons() {
	if s.caption != 0 {
		state := uintptr(0)
		if s.buttonsVisible {
			state = 4
		}
		showVideoWindow.Call(s.caption, state)
	}
}

func (s *Surface) SetCorners(radius float64, right bool) {
	if s.radius != radius || s.roundRight != right {
		s.radius, s.roundRight = radius, right
		s.updateRegion()
	}
}

func (s *Surface) updateRegion() {
	w, h := s.last[2], s.last[3]
	if w <= 0 || h <= 0 {
		return
	}
	diameter := int32(math.Round(s.radius * s.scale() * 2))
	if diameter <= 0 {
		setVideoRegion.Call(s.view, 0, 1)
		return
	}
	region, _, _ := createVideoRoundRegion.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(diameter), uintptr(diameter))
	if region == 0 {
		return
	}
	if !s.roundRight {
		right, _, _ := createVideoRectRegion.Call(uintptr(max(0, w-diameter)), 0, uintptr(w+1), uintptr(h+1))
		if right != 0 {
			combineVideoRegion.Call(region, region, right, 2)
			deleteVideoRegion.Call(right)
		}
	}
	if ok, _, _ := setVideoRegion.Call(s.view, region, 1); ok == 0 {
		deleteVideoRegion.Call(region)
	}
}

func (s *Surface) Metrics() RenderMetrics {
	// D3D11 输出由 mpv 呈现，不能把解码帧数冒充已提交的画面帧数。
	return RenderMetrics{Resizes: s.resizes.Load(), MaxResizeMS: float64(s.maxResizeNS.Load()) / 1e6, Backend: "mpv-d3d11-window"}
}
