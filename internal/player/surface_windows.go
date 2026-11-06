//go:build windows && (amd64 || arm64)

package player

import (
	"fmt"
	"syscall"
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

// Surface composites libmpv's software output into the Windows UI.
type Surface struct {
	*softwareSurface
	window uintptr
}

func isVirtualMachine() bool { return false }

func NewSurface(window uintptr, backend ...string) (*Surface, error) {
	if window == 0 {
		return nil, fmt.Errorf("无法创建 Windows 视频容器：窗口句柄无效")
	}
	return &Surface{window: window, softwareSurface: newSoftwareSurface()}, nil
}

func (s *Surface) scale() float64 {
	if videoWindowDPI.Find() == nil {
		if dpi, _, _ := videoWindowDPI.Call(s.window); dpi > 0 {
			return float64(dpi) / 96
		}
	}
	return 1
}

func (s *Surface) SetBounds(_, _ float64, w, h, _ float64) bool {
	scale := s.scale()
	return s.resize(w*scale, h*scale)
}

func (s *Surface) Show(show bool) {
	visible, _, _ := videoWindowVisible.Call(s.window)
	iconic, _, _ := videoWindowIconic.Call(s.window)
	s.softwareSurface.Show(show && visible != 0 && iconic == 0)
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
