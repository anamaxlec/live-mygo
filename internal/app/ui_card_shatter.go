//go:build darwin || windows

package app

import (
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

const cardShatterDuration = 420 * time.Millisecond

type cardShatterState struct {
	start         time.Time
	bounds        ui.Rect
	room          platforms.Room
	status        string
	dot           ui.Color
	paused, muted bool
}

func (a *app) shatterCardView(c *ui.Context) {
	s := a.cardShatter
	if s.start.IsZero() {
		return
	}
	remaining := cardShatterDuration - c.Now().Sub(s.start)
	if remaining <= 0 || c.Preferences().ReduceMotion {
		a.cardShatter = cardShatterState{}
		return
	}
	if !c.Now().After(s.start) {
		c.After(time.Millisecond)
		return
	}
	c.After(remaining)
	p := colors(c.Theme())
	pause, volume := iconPause, iconVolume
	if s.paused {
		pause = iconPlay
	}
	if s.muted {
		volume = iconMuted
	}
	ui.Overlay(c, func() {
		nowPlayingCardPanel(c.Key("card-fragments"), p).Absolute().Left(s.bounds.X).Top(s.bounds.Y).
			Size(s.bounds.W, s.bounds.H).PassThrough().Role(ui.RoleNone).Cache().
			Effect(cardShatterEffect{start: s.start}).Children(func() {
			a.channelCardIdentity(c, s.room)
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(4, 4).Radius(2).Background(s.dot)
				ui.Text(c, s.status).FontSize(10).TextColor(p.muted).Grow(1)
				for i, glyph := range []*ui.SVG{iconClose, pause, volume, iconRefresh} {
					size := float32(26)
					if i == 0 {
						size = 24
					}
					ui.Box(c).Size(size, size).Center().Children(func() { ui.Icon(c, glyph).Size(17, 17).TextColor(p.muted) })
				}
			})
		})
	})
}

type cardShatterEffect struct{ start time.Time }

func (fx cardShatterEffect) EffectMargin() ui.Point { return ui.Point{} }

func (fx cardShatterEffect) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	t := float32(p.Now().Sub(fx.start)) / float32(cardShatterDuration)
	if t >= 1 {
		return
	}
	p.AnimationFrameRate(60)
	scale := p.Scale()
	if t <= 0 {
		p.ImageRegion(content, ui.Rect{X: inset.X, Y: inset.Y, W: box.W * scale, H: box.H * scale}, box, ui.ImageOptions{})
		return
	}
	// 从关闭按钮一侧开始碎开。碎片带着卡片上的原图和文字下落。
	rows := [...]float32{0, .21, .49, .73, 1}
	for row := 0; row < len(rows)-1; row++ {
		cols := [...]float32{0, .16, .33, .51, .68, .84, 1}
		shift := float32(row%2)*.055 - .026
		for col := 1; col < len(cols)-1; col++ {
			cols[col] += shift
		}
		for col := 0; col < len(cols)-1; col++ {
			x, y := cols[col]*box.W, rows[row]*box.H
			w, h := (cols[col+1]-cols[col])*box.W, (rows[row+1]-rows[row])*box.H
			drift := motionPart(t, .025*float32(len(cols)-2-col), .92)
			angle := float32((row*11+col*7)%19-9) * 3 * drift
			dx := (cols[col] - .45) * 44 * ui.EaseOut(drift)
			dy := 58*drift*drift - float32(2+row%3)*11*drift*(1-drift)
			src := ui.Rect{X: inset.X + x*scale, Y: inset.Y + y*scale, W: w * scale, H: h * scale}
			dst := ui.Rect{X: box.X + x + dx, Y: box.Y + y + dy, W: w * (1 - .35*drift), H: h * (1 - .35*drift)}
			tint := ui.RGBA(255, 255, 255, 1-ui.EaseInOut(motionPart(drift, .35, 1)))
			p.Transform(ui.Rotation(angle), dst.X+dst.W/2, dst.Y+dst.H/2, func() {
				p.ImageRegion(content, src, dst, ui.ImageOptions{Tint: tint})
			})
		}
	}
}
