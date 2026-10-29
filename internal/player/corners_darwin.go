//go:build darwin

package player

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
)

// 四个小型系统材质视图只覆盖视频圆角外侧。GL 视图和 context 保持原样，
// 背景由与主窗口相同的 BehindWindow Sidebar 材质生成。
func (s *Surface) initMaterialCorners(msgSend uintptr, canvas objc.ID) error {
	var initRect func(objc.ID, objc.SEL, rect) objc.ID
	purego.RegisterFunc(&initRect, msgSend)
	purego.RegisterFunc(&s.setImageSize, msgSend)
	// 在启动播放、后台字图任务之前生成一次；按 MyGo 自身的连续圆角绘制。
	shape := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Radius(16).Background(ui.Hex("#ffffff"))
	}, 32, 32, 4)
	for i := range s.cornerViews {
		mask := image.NewNRGBA(image.Rect(0, 0, 64, 64))
		ox, oy := (i%2)*64, (i/2)*64
		for y := range 64 {
			for x := range 64 {
				mask.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255 - shape.RGBAAt(ox+x, oy+y).A})
			}
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, mask); err != nil {
			return err
		}
		data := encoded.Bytes()
		nsData := objc.ID(objc.GetClass("NSData")).Send(objc.RegisterName("dataWithBytes:length:"), uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
		runtime.KeepAlive(data)
		img := objc.ID(objc.GetClass("NSImage")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithData:"), nsData)
		if img == 0 {
			return fmt.Errorf("无法生成视频圆角材质 %d", i)
		}
		s.cornerImages[i] = img
		effect := initRect(objc.ID(objc.GetClass("NSVisualEffectView")).Send(objc.RegisterName("alloc")), objc.RegisterName("initWithFrame:"), rect{Size: size{16, 16}})
		effect.Send(objc.RegisterName("setMaterial:"), uintptr(7))
		effect.Send(objc.RegisterName("setBlendingMode:"), uintptr(0))
		effect.Send(objc.RegisterName("setState:"), uintptr(1))
		effect.Send(objc.RegisterName("setMaskImage:"), img)
		effect.Send(objc.RegisterName("setHidden:"), true)
		s.parent.Send(objc.RegisterName("addSubview:positioned:relativeTo:"), effect, ^uintptr(0), canvas)
		s.cornerViews[i] = effect
	}
	return nil
}

func (s *Surface) SetCorners(radius float64, right bool) {
	if radius == s.cornerRadius && right == s.cornerRight {
		return
	}
	s.cornerRadius, s.cornerRight = radius, right
	if radius > 0 {
		for _, img := range s.cornerImages {
			s.setImageSize(img, objc.RegisterName("setSize:"), size{radius, radius})
		}
	}
	s.updateMaterialCorners()
}

func (s *Surface) updateMaterialCorners() {
	r := s.last
	n := s.cornerRadius
	for i, effect := range s.cornerViews {
		show := s.visible.Load() && n > 0 && (i%2 == 0 || s.cornerRight)
		if show {
			x, y := r.Origin.X, r.Origin.Y+r.Size.H-n
			if i%2 == 1 {
				x += r.Size.W - n
			}
			if i/2 == 1 {
				y = r.Origin.Y
			}
			if s.parent.Send(objc.RegisterName("isFlipped")) != 0 {
				y = r.Origin.Y
				if i/2 == 1 {
					y += r.Size.H - n
				}
			}
			s.setRect(effect, objc.RegisterName("setFrame:"), rect{Origin: point{x, y}, Size: size{n, n}})
		}
		effect.Send(objc.RegisterName("setHidden:"), !show)
	}
}
