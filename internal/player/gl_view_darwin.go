//go:build darwin

package player

import (
	"github.com/ebitengine/purego/objc"
	"sync"
)

var glSurfaces sync.Map
var synchronizedGLViewClass = sync.OnceValues(func() (objc.Class, error) {
	// AppKit 的自动 update/reshape 只登记最新请求。真正的 context.update
	// 由 UI 帧在 SetBounds 的同一把锁内应用，不与后台绘制争用 drawable。
	deferUpdate := func(self objc.ID, _ objc.SEL) {
		if value, ok := glSurfaces.Load(self); ok {
			s := value.(*Surface)
			s.drawableDirty.Store(true)
			s.resizePending.Store(true)
			if s.invalidate != nil {
				s.invalidate()
			}
		}
	}
	lockFocus := func(self objc.ID, cmd objc.SEL) {
		if value, ok := glSurfaces.Load(self); ok {
			s := value.(*Surface)
			s.lock(s.context)
		}
		self.SendSuper(cmd)
	}
	unlockFocus := func(self objc.ID, cmd objc.SEL) {
		self.SendSuper(cmd)
		if value, ok := glSurfaces.Load(self); ok {
			s := value.(*Surface)
			s.unlock(s.context)
		}
	}
	return objc.RegisterClass("LiveMyGoGLView", objc.GetClass("NSOpenGLView"), nil, nil, []objc.MethodDef{
		{Cmd: objc.RegisterName("update"), Fn: deferUpdate},
		{Cmd: objc.RegisterName("reshape"), Fn: deferUpdate},
		{Cmd: objc.RegisterName("lockFocus"), Fn: lockFocus},
		{Cmd: objc.RegisterName("unlockFocus"), Fn: unlockFocus},
	})
})
