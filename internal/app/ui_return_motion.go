//go:build darwin || windows

package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	returnToCardDuration = 440 * time.Millisecond
	returnCardCatchStart = 392 * time.Millisecond
	returnCardCatchDip   = 433 * time.Millisecond
	returnCardCatchPeak  = 490 * time.Millisecond
	returnCardCatchEnd   = 575 * time.Millisecond
	cardOpenDuration     = 380 * time.Millisecond
)

func (a *app) classicReturn() bool {
	return a.hasSideViewer() || a.library != nil && a.library.Preferences.ClassicReturn
}

func returnCardScale(elapsed time.Duration) float32 {
	t := float32(elapsed)
	compress := .024 * ui.EaseOut(motionPart(t, float32(returnCardCatchStart), float32(returnCardCatchDip)))
	rebound := .039 * ui.EaseOut(motionPart(t, float32(returnCardCatchDip), float32(returnCardCatchPeak)))
	settle := .015 * ui.EaseInOut(motionPart(t, float32(returnCardCatchPeak), float32(returnCardCatchEnd)))
	return 1 - compress + rebound - settle
}

type returnToCardEffect struct {
	start    time.Time
	duration time.Duration
	card     *ui.Rect
	reduced  bool
	opening  bool
	reversed bool
}

func (fx returnToCardEffect) EffectMargin() ui.Point { return ui.Point{} }

func (fx returnToCardEffect) progressAt(now time.Time) float32 {
	return max(0, min(1, float32(now.Sub(fx.start))/float32(fx.duration)))
}

func cardOpeningOpacity(progress float32) float32 {
	return ui.EaseOut(motionPart(progress, 0, .15))
}

type returnToCardShape struct {
	box, card    ui.Rect
	upper, lower float32
	top, bottom  float32
}

func cardShapeAt(box, card ui.Rect, t float32) returnToCardShape {
	upper := ui.EaseInOut(min(t/.93, 1))
	lower := ui.EaseInOut(min(t/.63, 1))
	return returnToCardShape{
		box: box, card: card, upper: upper, lower: lower,
		top:    box.Y + (card.Y-box.Y)*upper,
		bottom: box.Y + box.H + (card.Y+card.H-box.Y-box.H)*lower,
	}
}

func (s returnToCardShape) edges(v float32) (left, right float32) {
	bend := v * v * (3 - 2*v)
	pull := s.upper + (s.lower-s.upper)*bend
	// 左沿整条移向卡片，右沿仍随行高收拢。
	left = s.box.X + (s.card.X-s.box.X)*s.lower
	sourceRight := s.box.X + s.box.W
	right = sourceRight + (s.card.X+s.card.W-sourceRight)*pull
	return left, right
}

func (s returnToCardShape) mapRect(source ui.Rect) ui.Rect {
	top := (source.Y - s.box.Y) / s.box.H
	bottom := (source.Y + source.H - s.box.Y) / s.box.H
	left, right := s.edges((top + bottom) / 2)
	scaleX := (right - left) / s.box.W
	return ui.Rect{
		X: left + (source.X-s.box.X)*scaleX,
		Y: s.top + top*(s.bottom-s.top),
		W: source.W * scaleX,
		H: (bottom - top) * (s.bottom - s.top),
	}
}

func (fx returnToCardEffect) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	progress := fx.progressAt(p.Now())
	if progress >= 1 {
		return
	}
	t := progress
	opacity := 1 - ui.EaseInOut(motionPart(t, .84, 1))
	if fx.opening {
		t = 1 - progress
	}
	if fx.opening || fx.reversed {
		opacity = cardOpeningOpacity(1 - t)
	}
	if !fx.opening {
		p.AnimationFrameRate(60)
	}
	card := *fx.card
	scale := p.Scale()
	src := ui.Rect{X: inset.X, Y: inset.Y, W: box.W * scale, H: box.H * scale}
	if fx.reduced || card.W <= 1 || card.H <= 1 {
		// 减少动态效果时只做交接淡出，画面保持原位。
		p.ImageRegion(content, src, box, ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, 1-ui.EaseOut(t))})
		return
	}

	shape := cardShapeAt(box, card, t)
	if opacity <= 0 {
		return
	}
	tint := ui.ImageOptions{Tint: ui.RGBA(255, 255, 255, opacity)}
	if shape.upper == shape.lower {
		dst := box
		if shape.upper == 1 {
			dst = card
		}
		p.ImageRegion(content, src, dst, tint)
		return
	}

	// 下缘更早靠近卡片，横向按行高平滑过渡。每行共用同一张纹理，
	// 对齐物理像素后相邻行无缝相接，D3D 可以将它们合并绘制。
	first := int(math.Round(float64(shape.top * scale)))
	last := int(math.Round(float64(shape.bottom * scale)))
	rows := float32(last - first)
	if rows <= 0 {
		return
	}
	for y := first; y < last; y++ {
		v := float32(y-first) / rows
		left, right := shape.edges(v + .5/rows)
		from := ui.Rect{X: src.X, Y: src.Y + src.H*v, W: src.W, H: src.H / rows}
		to := ui.Rect{X: left, Y: float32(y) / scale, W: right - left, H: 1 / scale}
		p.ImageRegion(content, from, to, tint)
	}
}

func (a *app) returnShrinkView(c *ui.Context) {
	if !a.returnShrink {
		return
	}
	if a.page == "player" || a.classicReturn() || (a.stream == nil && a.targetRoom.ID == "") {
		a.returnShrink = false
		a.returnImmersive = false
		a.returnShrinkAt = time.Time{}
		a.returnFrame = nil
		return
	}
	if a.builtPage == "player" {
		// 标题栏收到点击时，这一帧的播放页已经建好。
		c.After(time.Millisecond)
		return
	}
	if a.returnShrinkAt.IsZero() {
		a.returnShrinkAt = c.Now()
	}
	duration := returnToCardDuration
	reduced := c.Preferences().ReduceMotion
	if reduced {
		duration = 140 * time.Millisecond
	}
	elapsed := c.Now().Sub(a.returnShrinkAt)
	remaining := duration - elapsed
	if remaining <= 0 {
		if !reduced && elapsed < returnCardCatchEnd {
			c.After(returnCardCatchEnd - elapsed)
			return
		}
		a.returnShrink = false
		a.returnImmersive = false
		a.returnShrinkAt = time.Time{}
		a.normalMinimum()
		return
	}
	c.After(remaining)
	if !reduced && elapsed < returnCardCatchStart {
		c.After(returnCardCatchStart - elapsed)
	}
	fx := returnToCardEffect{start: a.returnShrinkAt, duration: duration, card: &a.nowCardBounds, reduced: reduced, reversed: a.returnFromOpen}
	a.playerMotionSheet(c, fx, a.playerNavWidth, a.playerChatWidth, elapsed == 0)
}

func (a *app) openFromCard(c *ui.Context, bounds ui.Rect) {
	if a.page == "player" || !a.cardOpenAt.IsZero() || a.snap {
		return
	}
	a.returnShrink = false
	a.returnImmersive = false
	a.returnShrinkAt = time.Time{}
	a.rememberHomeCatalog()
	a.returnPage = a.page
	a.revealControls()
	if !a.classicReturn() && !c.Preferences().ReduceMotion && bounds.W > 0 && bounds.H > 0 {
		a.cardOpenAt = c.Now()
		a.cardOpenFrom = bounds
		if a.surface != nil {
			a.cardOpenFrames = a.surface.Metrics().RenderedFrames
		}
	} else {
		a.enterPlayerFromCard(false)
	}

	if a.pausedOnLeave {
		a.pausedOnLeave = false
		a.setPlaybackPaused(false, a.recoverCardPlayback)
	}
	if a.hasSideViewer() && a.sideViewer.pausedOnLeave {
		a.sideViewer.setPlaybackPaused(false, nil)
	}
	if !a.shown {
		a.recoverCardPlayback()
	}
}

func (a *app) recoverCardPlayback() {
	if a.win == nil || a.closing || a.busy || (a.page != "player" && a.cardOpenAt.IsZero()) || roomOfflineStatus(a.status) {
		return
	}
	if a.stream != nil {
		a.reloadStreamInPlace()
	} else if a.targetRoom.ID != "" && a.status == "已取消连接" {
		a.resolveRoom(a.targetRoom.Platform, a.targetRoom.ID, -1, roomOpenOptions{keepBrowsing: true})
	}
}

func (a *app) enterPlayerFromCard(animated bool) {
	fromMonitor := a.page == "monitor"
	a.page = "player"
	a.focusMode = true
	if fromMonitor {
		a.syncMonitorPlayback()
	}
	if animated {
		a.navReturn++
	} else {
		a.returnFrame = nil
		a.fitWindowToVideo(true)
	}
}

func (a *app) advanceCardOpen(c *ui.Context) {
	if a.cardOpenAt.IsZero() {
		return
	}
	elapsed := c.Now().Sub(a.cardOpenAt)
	if elapsed < cardOpenDuration {
		return
	}
	if a.page != "player" {
		a.enterPlayerFromCard(true)
	}
	// 原生输出接手前保留展开时的最后一帧。手动暂停时保留到恢复播放。
	waiting := a.waitingForVideo() || a.surface != nil && a.surface.Metrics().RenderedFrames <= a.cardOpenFrames
	if a.returnFrame != nil && waiting {
		if a.shown && a.stats.Paused {
			return
		}
		if elapsed < 1500*time.Millisecond {
			c.After(16 * time.Millisecond)
			return
		}
	}
	a.cardOpenAt = time.Time{}
	a.returnFrame = nil
	a.fitWindowToVideo(true)
}

func (a *app) cardOpening(c *ui.Context) bool {
	return a.page != "player" && !a.cardOpenAt.IsZero() && c.Now().After(a.cardOpenAt)
}

func (a *app) cardOpenChatWidth(c *ui.Context) float32 {
	if a.chatVisible() {
		return a.chatWidth(c, 0)
	}
	return 0
}

func (a *app) reverseCardOpen(c *ui.Context) {
	progress := max(0, min(1, float32(c.Now().Sub(a.cardOpenAt))/float32(cardOpenDuration)))
	a.playerNavWidth, a.playerChatWidth = 0, a.cardOpenChatWidth(c)
	a.finishBackToBrowse(nil)
	if a.returnShrink {
		a.returnFromOpen = true
		duration := returnToCardDuration
		if c.Preferences().ReduceMotion {
			duration = 140 * time.Millisecond
		}
		// 展开与收拢共用轮廓。把时钟拨到对应的收拢位置，继续当前这一帧。
		a.returnShrinkAt = c.Now().Add(-time.Duration((1 - progress) * float32(duration)))
	}
}

func (a *app) cardOpenView(c *ui.Context) {
	if a.page == "player" || a.cardOpenAt.IsZero() {
		return
	}
	// 只重绘纹理不会更新命中区域，展开时也要重建跟随轮廓的返回按钮。
	c.After(min(16*time.Millisecond, max(time.Millisecond, cardOpenDuration-c.Now().Sub(a.cardOpenAt))))
	fx := returnToCardEffect{start: a.cardOpenAt, duration: cardOpenDuration, card: &a.cardOpenFrom, opening: true}
	a.playerMotionSheet(c, fx, 0, a.cardOpenChatWidth(c), false)
}

func (a *app) playerMotionSheet(c *ui.Context, fx returnToCardEffect, nav, chat float32, initial bool) {
	width, height := c.Size()
	p := colors(c.Theme())
	header := a.watchHeaderTarget()
	if a.returnShrink && a.returnImmersive {
		header = 0
	}
	a.snap, a.snapNav, a.snapChat = true, nav, chat
	ui.Overlay(c, func() {
		sheet := ui.Box(c.Key("return-to-card")).Absolute().Left(0).Top(0).Size(width, height).Background(p.bg).Clip()
		if initial {
			// 交接的首帧直接绘制完整播放页，下一帧才开始取纹理变形。
			c.After(time.Millisecond)
		} else {
			sheet.Cache().Effect(fx)
		}
		sheet.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
				a.desktopSidebar(c, a.snapNav, p)
				ui.Box(c).Grow(1).MinWidth(0).Children(func() { a.contentView(c, header) })
			})
			if header > 0 {
				ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(watchHeaderHeight).Children(func() { a.watchHeader(c) })
			}
		})
		// 变形中的页面不接收点击，只开放随轮廓移动的返回按钮。
		ui.Box(c).Absolute().Fill().Role(ui.RoleNone)
		if fx.opening && fx.progressAt(c.Now()) >= .075 && width > 0 && height > 0 {
			shape := cardShapeAt(ui.Rect{W: width, H: height}, *fx.card, 1-fx.progressAt(c.Now()))
			back := shape.mapRect(ui.Rect{X: watchHeaderInset(c, nav), Y: (watchHeaderHeight - 30) / 2, W: 30, H: 30})
			button := ui.ButtonBase(c.Key("opening-back")).Absolute().Left(back.X).Top(back.Y).Size(back.W, back.H).
				Background(ui.Transparent).FocusRing(false).Label("返回列表 · Esc")
			// 按下时命中移动中的按钮，松开时它可能已经滑出了指针下方。
			if button.Pressed() || button.Clicked() {
				a.backToBrowse(c)
				c.After(time.Millisecond)
			}
		}
	})
	a.snap = false
}
