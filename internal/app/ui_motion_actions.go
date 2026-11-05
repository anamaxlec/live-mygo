//go:build darwin || windows

package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/platforms"
)

var actionProgress = []ui.Keyframe{{Value: 0}, {At: 960 * time.Millisecond, Value: 1, Ease: ui.Linear}}
var ticketProgress = []ui.Keyframe{{Value: 0}, {At: 760 * time.Millisecond, Value: 1, Ease: ui.Linear}}
var ticketPaper = ui.MustParseSVG([]byte(`<svg width="38" height="24" viewBox="0 0 38 24"><path d="M3 1H35Q37 1 37 3V7A5 5 0 0 0 37 17V21Q37 23 35 23H3Q1 23 1 21V17A5 5 0 0 0 1 7V3Q1 1 3 1Z" fill="currentColor"/><path d="M28 4v2m0 3v2m0 3v2m0 3v1" stroke="#ffffff" stroke-width="1.2" opacity=".7"/></svg>`))
var envelopeSeams = ui.MustParseSVG([]byte(`<svg width="68" height="40" viewBox="0 0 68 40"><rect x=".75" y=".75" width="66.5" height="38.5" rx="4" fill="none" stroke="currentColor" stroke-width="1.25"/><path d="M1 2 34 25 67 2M1 39 24 20M67 39 44 20" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linejoin="round"/></svg>`))
var envelopeFlap = ui.MustParseSVG([]byte(`<svg width="68" height="26" viewBox="0 0 68 26"><path d="M1 25 31 3Q34 1 37 3L67 25Z" fill="currentColor"/></svg>`))

func motionPart(t, from, to float32) float32 { return max(0, min(1, (t-from)/(to-from))) }

// 收藏是一张应援票：出票、翻面、盖心形章，再收回按钮。取消时票根向两边分开。
func favoriteTicket(c *ui.Context, b ui.Element, selected bool, ink ui.Color) {
	s := ui.Local(b, "ticket", func() favoriteReaction { return favoriteReaction{selected: selected} })
	if s.selected != selected {
		s.selected = selected
		s.run++
	}
	t := float32(1)
	if s.run > 0 && !c.Preferences().ReduceMotion {
		t = b.Keyframes("ticket-progress", ticketProgress, ui.KeyframeOptions{Run: s.run})
	}
	icon := iconHeart
	if selected {
		icon = iconHeartFilled
	}
	ui.Box(c.Key("heart-artwork")).Size(17, 17).Center().Children(func() {
		stamp := motionPart(t, .50, .68) * (1 - motionPart(t, .68, 1))
		ui.Icon(c, icon).Size(17, 17).TextColor(ink).Scale(1+stamp*.2, 1+stamp*.2)
		if t >= 1 {
			return
		}
		if selected {
			out, back := motionPart(t, 0, .30), motionPart(t, .65, 1)
			lift := out * (1 - back)
			flip := float32(math.Abs(math.Cos(float64(motionPart(t, .12, .42)) * math.Pi)))
			scale := (.28 + .72*lift) * (1 - back*.7)
			ui.Box(c.Key("cheer-ticket")).Absolute().Left(-10.5).Top(-3.5).Size(38, 24).PassThrough().
				Translate(4*float32(math.Sin(float64(t)*math.Pi)), -24*lift).Rotate(-22*(1-out)+8*lift-8*back).
				Scale(scale*max(.07, flip), scale).Opacity(min(1, out*3) * (1 - back)).Children(func() {
				ui.Icon(c, ticketPaper).Size(38, 24).TextColor(ink)
				ui.Icon(c, iconHeartFilled).Absolute().Left(11).Top(6).Size(12, 12).TextColor(ui.Hex("#ffffff")).
					Scale(.6+.4*motionPart(t, .4, .55), .6+.4*motionPart(t, .4, .55)).Rotate(-10)
			})
		} else {
			for i := 0; i < 2; i++ {
				side := float32(i*2 - 1)
				ui.Box(c.Key(i)).Absolute().Left(-2+float32(i)*11).Top(-1).Size(11, 20).Clip().PassThrough().
					Translate(side*14*t, 18*t*t).Rotate(side * 35 * t).Opacity(1 - motionPart(t, .15, .72)).Children(func() {
					ui.Icon(c, ticketPaper).Absolute().Left(-float32(i)*19).Size(38, 24).TextColor(ink)
				})
			}
		}
	})
}

type chatContentID uint64
type chatAreaID string

func (a *app) clearChatWithMotion() {
	a.clearChat()
	a.chatClearRun++
	a.chatClearActive = true
}

// 只清理当前列表的视觉快照；新到达的消息照常接收、显示和点击。
func (a *app) chatEnvelope(c *ui.Context, area ui.Element) {
	if !a.chatClearActive || c.Preferences().ReduceMotion {
		return
	}
	t := area.Keyframes("chat-envelope", actionProgress, ui.KeyframeOptions{Run: a.chatClearRun})
	if t >= 1 {
		return
	}
	p := colors(c.Theme())
	flight := motionPart(t, .70, 1)
	open := 1 - motionPart(t, .43, .65)
	ui.Box(c.Key("chat-envelope")).Absolute().LeftPercent(60).TopPercent(80).Size(68, 40).PassThrough().
		Translate(-34+flight*170, -20-flight*flight*95).Rotate(-7*(1-motionPart(t, 0, .4))+flight*24).
		Scale(.82+.18*motionPart(t, 0, .2), .82+.18*motionPart(t, 0, .2)).Opacity(1 - flight).Children(func() {
		ui.Icon(c, envelopeFlap).Absolute().Top(-24*open).Size(68, 26).TextColor(p.accent.Alpha(.24)).Scale(1, open)
		ui.Box(c).Absolute().Top(0).Size(68, 40).Radius(4).Background(p.bg).Border(1, p.accent.Alpha(.6))
		for i := 0; i < 3; i++ {
			ui.Box(c.Key(i)).Absolute().Left(9+float32(i)*2).Top(-8*open+float32(i)*3).Size(47, 21).Radius(2).Background(p.soft).Border(.7, p.line).Rotate(float32(i-1)*4).Scale(1, open)
		}
		ui.Box(c).Absolute().Top(12).Size(68, 28).Radius(0, 0, 4, 4).Background(p.bg)
		ui.Icon(c, envelopeSeams).Absolute().Size(68, 40).TextColor(p.accent.Alpha(.5))
		ui.Icon(c, envelopeFlap).Absolute().Top(0).Size(68, 26).TextColor(p.accent.Alpha(.18)).Scale(1, -(1-open)).TransformOrigin(.5, 0).Translate(0, 26)
		ui.Icon(c, iconCheck).Absolute().Left(27).Top(15).Size(14, 14).TextColor(p.accent).Opacity(1 - open).Rotate(-12)
	})
}

type channelCardState struct {
	room, previous platforms.Room
	run            uint64
}

var channelProgress = []ui.Keyframe{{Value: 0}, {At: 520 * time.Millisecond, Value: 1, Ease: ui.Linear}}

// 封面像换台的电视：上下快门合拢，信号线掠过，再打开新频道。
func (a *app) channelCardIdentity(c *ui.Context, room platforms.Room) {
	p := colors(c.Theme())
	row := ui.Row(c.Key("channel-identity")).FillWidth().Gap(9).AlignItems(ui.Center)
	s := ui.Local(row, "channel-state", func() channelCardState { return channelCardState{room: room} })
	old := s.room
	if roomKey(room) != roomKey(s.room) {
		s.run++
		s.room = room
		s.previous = old
	}
	t := float32(1)
	if s.run > 0 && !c.Preferences().ReduceMotion {
		t = row.Keyframes("channel-tune", channelProgress, ui.KeyframeOptions{Run: s.run})
	}
	gate := motionPart(t, 0, .24) * (1 - motionPart(t, .48, 1))
	display := room
	if t < .4 && s.previous.ID != "" {
		display = s.previous
	}
	row.Children(func() {
		ui.Box(c).Size(40, 40).Radius(7).Clip().Background(p.soft).Children(func() {
			a.coverImage(c, display.Cover, 7)
			if t < 1 {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Height(20 * gate).Background(p.text)
				ui.Box(c).Absolute().Bottom(0).Left(0).Right(0).Height(20 * gate).Background(p.text)
				ui.Box(c).Absolute().Top(19.4).Left(4 + 7*float32(math.Sin(float64(t)*math.Pi))).Right(4).Height(1.2).Background(p.accent).Opacity(gate)
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(4).Clip().Children(func() {
			ui.Text(c, a.displayRoomName(display)).FontSize(12).FontWeight(600).TextColor(p.text).SingleLine().Translate(0, gate*6).Opacity(1 - gate*.75)
			ui.Text(c, display.Title).FontSize(10.5).TextColor(p.muted).SingleLine().Translate(0, gate*10).Opacity(1 - gate)
		})
	})
}
