//go:build windows

package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

const (
	appWindowGetFromWindowID = 9  // IAppWindowStatics.GetFromWindowId
	appWindowSetPresenter    = 26 // IAppWindow.SetPresenter(AppWindowPresenterKind)
	presenterDefault         = 0
	presenterCompactOverlay  = 1
)

var appWindowStaticsID = syscall.GUID{Data1: 0x3c315c24, Data2: 0xd540, Data3: 0x5d72, Data4: [8]byte{0xb5, 0x18, 0xb2, 0x26, 0xb8, 0x36, 0x27, 0xcb}}

var pictureKernel = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "kernel32.dll"))
var pictureCOM = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "combase.dll"))

type pictureWinRTObject struct{ vtable *[27]uintptr }

func (o *pictureWinRTObject) call(slot int, arg ...uintptr) uintptr {
	args := append([]uintptr{uintptr(unsafe.Pointer(o))}, arg...)
	hr, _, _ := syscall.SyscallN(o.vtable[slot], args...)
	return hr
}

type windowsSystemPicture struct {
	app    *app
	window *pictureWinRTObject
}

func (a *app) startSystemPicture() (systemPictureInPicture, error) {
	window, err := getPictureAppWindow(a.win.NativeHandle())
	if err != nil {
		return nil, err
	}
	a.toggleCompanion()
	if hr := window.call(appWindowSetPresenter, presenterCompactOverlay); int32(hr) < 0 {
		window.call(2)
		a.toggleCompanion()
		return nil, pictureHRESULT("无法打开系统画中画", hr)
	}
	return &windowsSystemPicture{app: a, window: window}, nil
}

func (p *windowsSystemPicture) SetPaused(bool) {}

func (p *windowsSystemPicture) Close() {
	hr := p.window.call(appWindowSetPresenter, presenterDefault)
	p.window.call(2)
	if p.app.companion {
		p.app.toggleCompanion()
	}
	if int32(hr) < 0 {
		p.app.toast = pictureHRESULT("无法退出系统画中画", hr).Error()
	}
}

func getPictureAppWindow(hwnd uintptr) (*pictureWinRTObject, error) {
	getID, err := pictureWindowIDProc()
	if err != nil {
		return nil, err
	}
	var windowID uint64
	hr, _, _ := getID.Call(hwnd, uintptr(unsafe.Pointer(&windowID)))
	if int32(hr) < 0 {
		return nil, pictureHRESULT("无法取得播放器窗口", hr)
	}
	name, _ := syscall.UTF16FromString("Microsoft.UI.Windowing.AppWindow")
	var hstr uintptr
	hr, _, _ = pictureCOM.NewProc("WindowsCreateString").Call(uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&hstr)))
	if int32(hr) < 0 {
		return nil, pictureHRESULT("无法访问系统窗口接口", hr)
	}
	defer pictureCOM.NewProc("WindowsDeleteString").Call(hstr)
	var statics *pictureWinRTObject
	hr, _, _ = pictureCOM.NewProc("RoGetActivationFactory").Call(hstr, uintptr(unsafe.Pointer(&appWindowStaticsID)), uintptr(unsafe.Pointer(&statics)))
	if int32(hr) < 0 || statics == nil {
		return nil, pictureHRESULT("无法访问系统窗口接口", hr)
	}
	defer statics.call(2)
	var window *pictureWinRTObject
	// Keep the out pointer on a stable address across the WinRT call.
	hr, _, _ = syscall.SyscallN(statics.vtable[appWindowGetFromWindowID], uintptr(unsafe.Pointer(statics)), uintptr(windowID), uintptr(unsafe.Pointer(&window)))
	if int32(hr) < 0 || window == nil {
		return nil, pictureHRESULT("无法取得播放器窗口", hr)
	}
	return window, nil
}

var pictureWindowIDProc = sync.OnceValues(func() (*syscall.Proc, error) {
	create := pictureKernel.NewProc("TryCreatePackageDependency")
	add := pictureKernel.NewProc("AddPackageDependency")
	if create.Find() != nil || add.Find() != nil {
		return nil, errors.New("系统画中画需要 Windows 11")
	}
	// Windows App SDK 2.x shares a single Microsoft-signed framework family.
	family, _ := syscall.UTF16PtrFromString("Microsoft.WindowsAppRuntime.2_8wekyb3d8bbwe")
	var dependency *uint16
	hr, _, _ := create.Call(0, uintptr(unsafe.Pointer(family)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&dependency)))
	if int32(hr) < 0 || dependency == nil {
		return nil, errors.New("系统画中画需要 Windows App Runtime 2.x，安装后请重启 Live MyGo")
	}
	heap, _, _ := pictureKernel.NewProc("GetProcessHeap").Call()
	free := pictureKernel.NewProc("HeapFree")
	defer free.Call(heap, 0, uintptr(unsafe.Pointer(dependency)))
	var context uintptr
	var packageName *uint16
	hr, _, _ = add.Call(uintptr(unsafe.Pointer(dependency)), 0, 0, uintptr(unsafe.Pointer(&context)), uintptr(unsafe.Pointer(&packageName)))
	if int32(hr) < 0 || packageName == nil {
		return nil, pictureHRESULT("无法加载 Windows App Runtime", hr)
	}
	defer free.Call(heap, 0, uintptr(unsafe.Pointer(packageName)))
	getPath := pictureKernel.NewProc("GetPackagePathByFullName")
	var size uint32
	getPath.Call(uintptr(unsafe.Pointer(packageName)), uintptr(unsafe.Pointer(&size)), 0)
	if size == 0 {
		return nil, errors.New("无法定位 Windows App Runtime")
	}
	path := make([]uint16, size)
	result, _, _ := getPath.Call(uintptr(unsafe.Pointer(packageName)), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&path[0])))
	if result != 0 {
		return nil, fmt.Errorf("无法定位 Windows App Runtime：%d", result)
	}
	library, err := syscall.LoadDLL(filepath.Join(syscall.UTF16ToString(path), "Microsoft.Internal.FrameworkUdk.dll"))
	if err != nil {
		return nil, errors.New("无法加载 Windows App Runtime 窗口组件")
	}
	proc, err := library.FindProc("Windowing_GetWindowIdFromWindow")
	if err != nil {
		return nil, errors.New("Windows App Runtime 未提供窗口接口")
	}
	return proc, nil
})

func pictureHRESULT(message string, hr uintptr) error {
	return fmt.Errorf("%s (0x%08X)", message, uint32(hr))
}
