//go:build darwin

package app

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

type flyingCaption struct {
	ID            uint64
	Text          string
	Color         uint32
	Lane          int
	Width, Height float32
	Started       time.Time
	Sprite        *captionSprite
	NextSprite    *captionSprite
}

// 只比较影响字图的实际样式，轨道与速度变化不触发文字栅格化。
type captionStyle struct {
	font, color           string
	size, stroke, opacity float64
	weight                int
}

func spriteStyle(p library.Preferences) captionStyle {
	return captionStyle{p.DanmakuFont, p.DanmakuColor, p.DanmakuSize, p.DanmakuStroke, p.DanmakuOpacity, p.DanmakuWeight}
}

// 每条弹幕复用当前样式的 2× 字图，正常动画帧仅移动一张纹理。
// 字体栅格化留在单个后台槽中，避免 MyGo 共享字形图集被并发读写。
type captionSprite struct {
	bitmap        atomic.Pointer[ui.Bitmap]
	width, height float32
	style         captionStyle
}

func renderCaptionSprite(text string, w, h float32, color ui.Color, prefs library.Preferences) *ui.Bitmap {
	const pad float32 = 4
	img := ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintCaption(p, r.X+pad, r.Y+pad, text, color, prefs) })
	}, int(math.Ceil(float64(w+pad*2))), int(math.Ceil(float64(h+pad*2))), 2)
	return ui.NewBitmap(img)
}
func (a *app) captionSprite(text string, w, h float32, color ui.Color, prefs library.Preferences) *captionSprite {
	sprite := &captionSprite{width: float32(math.Ceil(float64(w + 8))), height: float32(math.Ceil(float64(h + 8))), style: spriteStyle(prefs)}
	if a.ctx == nil {
		sprite.bitmap.Store(renderCaptionSprite(text, w, h, color, prefs))
		return sprite
	}
	if a.captionSlots == nil {
		a.captionSlots = make(chan struct{}, 1)
	}
	slots := a.captionSlots
	go func() {
		select {
		case slots <- struct{}{}:
		case <-a.ctx.Done():
			return
		}
		defer func() { <-slots }()
		sprite.bitmap.Store(renderCaptionSprite(text, w, h, color, prefs))
	}()
	return sprite
}

func videoContentRect(width, height, aspect float32, mode string) ui.Rect {
	r := ui.Rect{W: width, H: height}
	if aspect <= 0 {
		aspect = 16.0 / 9
	}
	if mode == "填满" || mode == "拉伸" {
		return r
	}
	if width/height > aspect {
		r.W = height * aspect
		r.X = (width - r.W) / 2
	} else {
		r.H = width / aspect
		r.Y = (height - r.H) / 2
	}
	return r
}
func captionX(item flyingCaption, now time.Time, width, speed float32) float32 {
	return width - float32(now.Sub(item.Started).Seconds())*160*(width/1280)*speed
}
func captionColor(value uint32, choice string) ui.Color {
	switch choice {
	case "白色":
		return ui.Hex("#ffffff")
	case "暖黄":
		return ui.Hex("#ffe38a")
	case "浅蓝":
		return ui.Hex("#a9d5ff")
	case "粉色":
		return ui.Hex("#ffbbd3")
	}
	if value == 0 {
		value = 0xffffff
	}
	return ui.RGB(uint8(value>>16), uint8(value>>8), uint8(value))
}
func captionSpan(text string, prefs library.Preferences, color ui.Color) ui.Span {
	return ui.Span{Text: text, Font: prefs.DanmakuFont, Size: float32(prefs.DanmakuSize), Weight: prefs.DanmakuWeight, Color: color}
}
func paintCaption(p *ui.Painter, x, y float32, text string, color ui.Color, prefs library.Preferences) {
	opacity := float32(prefs.DanmakuOpacity / 100)
	stroke := float32(prefs.DanmakuStroke)
	if stroke > 0 {
		ink := ui.RGBA(8, 8, 12, .9*opacity)
		outline := captionSpan(text, prefs, ink)
		for _, offset := range [][2]float32{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-.7, -.7}, {.7, -.7}, {-.7, .7}, {.7, .7}} {
			p.RichText(x+offset[0]*stroke, y+offset[1]*stroke, 0, outline)
		}
	}
	p.RichText(x, y, 0, captionSpan(text, prefs, color.Alpha(opacity)))
}

// 弹幕由 MyGo 的独立绘制层驱动，不再每帧更新 mpv 字幕或重绘直播画面。
func (a *app) barrageView(c *ui.Context, width, height float32) {
	if a.library == nil || !a.barrageVisible() || !a.shown || a.minimized {
		return
	}
	prefs := a.library.Preferences
	aspect := float32(16.0 / 9)
	if a.stats.Width > 0 && a.stats.Height > 0 {
		aspect = float32(a.stats.Width) / float32(a.stats.Height)
	}
	area := videoContentRect(width, height, aspect, a.fitMode)
	now := c.Now()
	speed := float32(prefs.DanmakuSpeed)
	if speed <= 0 {
		speed = 1
	}
	active := a.captions[:0]
	for _, item := range a.captions {
		if captionX(item, now, area.W, speed)+item.Width >= 0 {
			if item.Sprite == nil || item.Sprite.style != spriteStyle(prefs) {
				item.Width, item.Height = c.MeasureText(0, captionSpan(item.Text, prefs, ui.Hex("#ffffff")))
				if item.NextSprite != nil && item.NextSprite.style == spriteStyle(prefs) && item.NextSprite.bitmap.Load() != nil {
					item.Sprite, item.NextSprite = item.NextSprite, nil
				} else if a.preferencesDeadline.IsZero() && (item.NextSprite == nil || item.NextSprite.style != spriteStyle(prefs)) {
					// 拖动滑块时即时绘字，停下后才生成最终字图，不积压每一帧的栅格化任务。
					item.NextSprite = a.captionSprite(item.Text, item.Width, item.Height, captionColor(item.Color, prefs.DanmakuColor), prefs)
				}
			}
			active = append(active, item)
		}
	}
	a.captions = active
	top, bottom, gap := float32(prefs.DanmakuTop), float32(prefs.DanmakuBottom), float32(prefs.DanmakuGap)
	_, lineHeight := c.MeasureText(0, captionSpan("字Ag", prefs, ui.Hex("#ffffff")))
	lanes := max(1, min(max(1, prefs.DanmakuLines), int((area.H*float32(prefs.DanmakuArea)-top-bottom)/(lineHeight+gap))))
	for _, msg := range a.pendingCaptions {
		if len(a.captions) >= 64 {
			break
		}
		runes := []rune(msg.Text)
		if len(runes) > 80 {
			runes = append(runes[:80], '…')
		}
		text := strings.ReplaceAll(string(runes), "\n", " ")
		if prefs.DanmakuMerge {
			duplicate := false
			for _, item := range a.captions {
				if strings.EqualFold(strings.TrimSpace(item.Text), strings.TrimSpace(text)) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
		}
		w, h := c.MeasureText(0, captionSpan(text, prefs, ui.Hex("#ffffff")))
		lane := -1
		for i := 0; i < lanes; i++ {
			free := true
			for _, item := range a.captions {
				if item.Lane == i && captionX(item, now, area.W, speed)+item.Width+36 > area.W {
					free = false
					break
				}
			}
			if free {
				lane = i
				break
			}
		}
		if lane >= 0 {
			a.captions = append(a.captions, flyingCaption{ID: msg.ID, Text: text, Color: msg.Color, Lane: lane, Width: w, Height: h, Started: now, Sprite: a.captionSprite(text, w, h, captionColor(msg.Color, prefs.DanmakuColor), prefs)})
		}
	}
	a.pendingCaptions = nil
	items := append([]flyingCaption(nil), a.captions...)
	if len(items) == 0 {
		return
	}
	ui.Box(c.Key("native-danmaku")).Absolute().Left(area.X).Top(area.Y).Width(area.W).Height(area.H).Clip().Role(ui.RoleNone).Draw(func(p *ui.Painter, r ui.Rect) {
		moving := false
		for _, item := range items {
			x := captionX(item, p.Now(), r.W, speed)
			if x+item.Width < 0 {
				continue
			}
			y := top + float32(item.Lane)*(lineHeight+gap)
			if y+item.Height > r.H-bottom {
				continue
			}
			moving = true
			sprite := item.Sprite
			if item.NextSprite != nil && item.NextSprite.style == spriteStyle(prefs) && item.NextSprite.bitmap.Load() != nil {
				sprite = item.NextSprite
			}
			if sprite != nil && sprite.style == spriteStyle(prefs) && sprite.bitmap.Load() != nil {
				p.Image(sprite.bitmap.Load(), ui.Rect{X: r.X + x - 4, Y: r.Y + y - 4, W: sprite.width, H: sprite.height}, ui.Contain)
			} else {
				p.Transform(ui.Translation(x, y), 0, 0, func() { paintCaption(p, r.X, r.Y, item.Text, captionColor(item.Color, prefs.DanmakuColor), prefs) })
			}
		}
		if moving {
			if prefs.DanmakuFPS >= 120 {
				p.AnimationFrame()
			} else {
				p.AnimationFrameRate(float32(max(30, prefs.DanmakuFPS)))
			}
		}
	})
}

func (a *app) queueCaptions(messages []platforms.ChatMessage) {
	if !a.barrageVisible() || !a.shown || a.page != "player" || a.minimized {
		return
	}
	for _, msg := range messages {
		if msg.Kind != "superchat" && len(a.pendingCaptions) < 64 {
			a.pendingCaptions = append(a.pendingCaptions, msg)
		}
	}
}
func (a *app) clearFlyingChat() { a.captions = nil; a.pendingCaptions = nil }

func (a *app) updateCaptionPreferences(before, after library.Preferences) {
	if before.Danmaku && !after.Danmaku {
		a.clearFlyingChat()
		return
	}
	if before.DanmakuSpeed != after.DanmakuSpeed {
		oldSpeed, newSpeed := before.DanmakuSpeed, after.DanmakuSpeed
		if oldSpeed <= 0 {
			oldSpeed = 1
		}
		if newSpeed <= 0 {
			newSpeed = 1
		}
		now := time.Now()
		for i := range a.captions {
			elapsed := now.Sub(a.captions[i].Started)
			a.captions[i].Started = now.Add(-time.Duration(float64(elapsed) * oldSpeed / newSpeed))
		}
	}
}
