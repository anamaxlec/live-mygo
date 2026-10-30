//go:build darwin || windows

package app

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"time"

	"github.com/egoist/mygo/ui"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
)

type recentChat struct {
	Text string
	At   time.Time
}

func (a *app) chatVisible() bool { return a.library != nil && a.library.Preferences.ChatPanel }
func (a *app) barrageVisible() bool {
	return a.targetRoom.Platform != platforms.FLive && a.library != nil && a.library.Preferences.Danmaku
}

func (a *app) toggleChat() {
	if a.library != nil {
		next := a.library.Preferences
		next.ChatPanel = !next.ChatPanel
		a.savePreferences(next)
		a.revealControls()
		a.fitWindowToVideo(false)
	}
}
func (a *app) toggleBarrage() {
	if a.targetRoom.Platform == platforms.FLive {
		return
	}
	if a.library != nil {
		next := a.library.Preferences
		next.Danmaku = !next.Danmaku
		a.savePreferences(next)
		a.revealControls()
	}
}

func (a *app) startChat(force bool) {
	if a.stream == nil {
		return
	}
	room := a.stream.Room
	if room.Platform == platforms.FLive {
		if a.chatCancel != nil {
			a.chatCancel()
			a.chatCancel = nil
		}
		a.chatGeneration++
		a.chatRoomKey = roomKey(room)
		a.chatStatus = ""
		a.clearChat()
		return
	}
	key := string(room.Platform) + ":" + room.ID
	if !force && a.chatRoomKey == key && a.chatCancel != nil && (a.chatStatus == "已连接" || a.chatStatus == "正在连接") {
		return
	}
	if a.chatCancel != nil {
		a.chatCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.chatCancel = cancel
	a.chatGeneration++
	gen := a.chatGeneration
	if a.chatRoomKey != key {
		a.clearChat()
		a.chatDropped = 0
	}
	a.chatRoomKey = key
	a.chatStatus = "正在连接"
	// 醒目留言独立排队，普通弹幕高峰不会把它们挤出接收队列。
	events, priority := make(chan platforms.ChatMessage, 256), make(chan platforms.ChatMessage, 128)
	var dropped atomic.Uint64
	postStatus := func(status string) {
		a.win.Update(func() {
			if !a.closing && a.chatGeneration == gen {
				a.chatStatus = status
			}
		})
	}
	go func() {
		defer cancel()
		reconnectChat(ctx, func(connected func()) error {
			return a.client.ListenChat(ctx, room.Platform, room.ID, func(v platforms.ChatMessage) {
				queue := events
				if v.Kind == "superchat" {
					queue = priority
				}
				select {
				case queue <- v:
				default:
					dropped.Add(1)
				}
			}, func(status string) {
				if status == "已连接" {
					connected()
				}
				postStatus(status)
			})
		}, postStatus, chatRetryDelay)
	}()
	go func() {
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		for {
			var first platforms.ChatMessage
			select {
			case <-ctx.Done():
				return
			case first = <-priority:
			case first = <-events:
			}
			timer.Reset(80 * time.Millisecond)
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			batch := []platforms.ChatMessage{first}
			for len(batch) < 384 {
				select {
				case msg := <-priority:
					batch = append(batch, msg)
					continue
				default:
				}
				select {
				case msg := <-events:
					batch = append(batch, msg)
				default:
					goto drained
				}
			}
		drained:
			lost := dropped.Swap(0)
			a.win.Update(func() { a.acceptChatMessages(gen, room.Platform, batch, lost) })
		}
	}()
}

// 先作废旧连接的回调，再取消网络订阅；解析新房间失败也不会混入旧消息。
func (a *app) resetChat(status string) {
	a.chatGeneration++
	if a.chatCancel != nil {
		a.chatCancel()
		a.chatCancel = nil
	}
	a.chatRoomKey = ""
	a.chatStatus = status
	a.chatDropped = 0
	a.clearChat()
}

func (a *app) acceptChatMessages(gen int, platform platforms.Platform, batch []platforms.ChatMessage, dropped uint64) {
	if a.closing || gen != a.chatGeneration {
		return
	}
	a.chatDropped += dropped
	accepted := make([]platforms.ChatMessage, 0, len(batch))
	for _, msg := range batch {
		if a.library != nil && a.library.Preferences.Blocks(platform, msg) {
			continue
		}
		key := strings.ToLower(strings.Join(strings.Fields(msg.Text), " "))
		duplicate := false
		if msg.Kind != "superchat" && a.library != nil && a.library.Preferences.FilterDuplicates {
			for _, last := range a.chatRecent {
				if key == last.Text && time.Since(last.At) < 10*time.Second {
					duplicate = true
					break
				}
			}
		}
		if duplicate {
			continue
		}
		if msg.Kind != "superchat" {
			a.chatRecent = append(a.chatRecent, recentChat{Text: key, At: time.Now()})
			if len(a.chatRecent) > 50 {
				a.chatRecent = a.chatRecent[len(a.chatRecent)-50:]
			}
		}
		a.chatNext++
		msg.ID = a.chatNext
		msg.Received = time.Now()
		accepted = append(accepted, msg)
	}
	a.chatMessages = retainChat(a.chatMessages, accepted)
	a.queueCaptions(accepted)
}

// 普通消息与醒目留言分别保留 500 / 100 条，并保持抵达顺序与滚动锚点。
func retainChat(previous, batch []platforms.ChatMessage) []platforms.ChatMessage {
	all := append(previous, batch...)
	normal, special := 0, 0
	kept := make([]platforms.ChatMessage, 0, min(len(all), 600))
	for i := len(all) - 1; i >= 0; i-- {
		if all[i].Kind == "superchat" {
			special++
			if special > 100 {
				continue
			}
		} else {
			normal++
			if normal > 500 {
				continue
			}
		}
		kept = append(kept, all[i])
	}
	slices.Reverse(kept)
	return kept
}

func (a *app) chatView(c *ui.Context) {
	p := colors(c.Theme())
	prefs := library.Preferences{ChatConnection: true}
	if a.library != nil {
		prefs = a.library.Preferences
	}
	backdrop := p.bg
	visible := make([]int, 0, len(a.chatMessages))
	for i, msg := range a.chatMessages {
		if a.chatOnlySC && msg.Kind != "superchat" {
			continue
		}
		if a.library != nil && a.library.Preferences.Blocks(a.targetRoom.Platform, msg) {
			continue
		}
		visible = append(visible, i)
	}
	if a.chatList.AtEnd() && a.chatList.FollowEnd {
		a.chatSeen = a.chatNext
	}
	a.chatList.Key = func(i int) any { return a.chatMessages[visible[i]].ID }
	radius := watchFrameRadius - watchFrameBorder
	if a.win != nil && a.win.IsFullScreen() {
		radius = 0
	}
	// 白底自身也收圆角，避免原生合成路径只裁切子内容时留下白色方角。
	listLabel := "弹幕列表"
	if a.targetRoom.Platform == platforms.FLive {
		listLabel = "播放线路列表"
	}
	ui.Column(c.Key("docked-chat")).FillWidth().FillHeight().Shrink(0).Radius(0, radius, radius, 0).Clip().Background(backdrop).BorderWidth(0, 0, 0, .5).BorderColor(p.line).Label(listLabel).Children(func() {
		ui.Row(c).Height(44).Shrink(0).Padding(0, 11, 0, 14).Gap(5).AlignItems(ui.Center).Children(func() {
			if a.targetRoom.Platform == platforms.FLive {
				ui.Text(c, "播放线路").FontSize(12).FontWeight(600).TextColor(p.text)
				return
			}
			if a.targetRoom.Platform == platforms.Bilibili || a.targetRoom.Platform == platforms.Douyu {
				for _, onlySC := range []bool{false, true} {
					label := "全部"
					if onlySC {
						label = "醒目留言"
					}
					if filterButton(c.Key(onlySC), label, a.chatOnlySC == onlySC).Height(24).Clicked() {
						a.chatOnlySC = onlySC
						a.chatList = ui.ListState{FollowEnd: true}
					}
				}
			}
			ui.Box(c).Grow(1)
			settings := iconButton(c.Key("chat-settings"), iconSettings, "弹幕列表设置")
			if settings.Clicked() {
				a.chatSettingsOpen = !a.chatSettingsOpen
			}
			a.chatSettingsPopover(c, settings)
			more := iconButton(c.Key("chat-actions"), iconMore, "弹幕列表操作")
			more.Menu(func(m *ui.Menu) {
				if a.chatDropped > 0 {
					m.Item(fmt.Sprintf("高峰队列已省略 %d 条消息", a.chatDropped)).Disabled(true)
					m.Separator()
				}
				if m.Item("自动滚动").Checked(a.chatList.FollowEnd).Chosen() {
					a.chatList.FollowEnd = !a.chatList.FollowEnd
					if a.chatList.FollowEnd {
						a.chatList.ScrollToEnd()
					}
				}
				if m.Item("重新连接弹幕").Chosen() {
					a.startChat(true)
				}
				m.Separator()
				if m.Item("复制可见消息").Disabled(len(visible) == 0).Chosen() {
					var lines []string
					for _, i := range visible {
						msg := a.chatMessages[i]
						lines = append(lines, msg.User+"："+msg.Text)
					}
					mygo.Clipboard.WriteText(strings.Join(lines, "\n"))
					a.toast = "已复制弹幕"
				}
				if m.Item("清空当前弹幕").Disabled(len(a.chatMessages) == 0).Chosen() {
					a.clearChat()
				}
				m.Separator()
				if m.Item("关键词与用户屏蔽").Chosen() {
					a.filterWindow()
				}
				if m.Item("收起弹幕列表").Chosen() {
					a.toggleChat()
				}
			})
		})
		ui.Box(c).Height(1).Shrink(0).Background(p.line)
		area := ui.Box(c.Key("chat-list-area")).Grow(1).MinHeight(0)
		target := float32(0)
		if prefs.ChatConnection && a.targetRoom.ID != "" && a.connectionPending() {
			target = 1
		}
		opacity := area.AnimateTo("connection-opacity", target, ui.Animation{Duration: 180 * time.Millisecond, Ease: ui.EaseOut})
		area.Children(func() {
			if a.targetRoom.Platform == platforms.FLive {
				a.raceSourceList(c)
			} else if len(visible) > 0 {
				ui.List(c, &a.chatList, len(visible), func(i int) { a.chatBubble(c, a.chatMessages[visible[i]]) }).Fill().PaddingY(9)
			} else {
				a.chatEmptyView(c)
			}
			if len(visible) > 0 && (!a.chatList.AtEnd() || !a.chatList.FollowEnd) {
				ui.Row(c).Absolute().Bottom(12).Left(0).Right(0).Justify(ui.Center).Children(func() {
					label := "回到最新弹幕"
					if count := a.chatNext - a.chatSeen; count > 0 {
						label = fmt.Sprintf("%d 条新弹幕", count)
					}
					b := ui.ButtonBase(c.Key("chat-latest")).Height(30).PaddingX(13).Gap(6).Radius(15).Background(p.bg).Border(1, p.line).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, .09)).AlignItems(ui.Center).Label("回到最新弹幕")
					b.Children(func() {
						ui.Icon(c, iconChevronDown).Size(13, 13).TextColor(p.accent)
						ui.Text(c, label).FontSize(11).FontWeight(500).TextColor(p.text)
					})
					if b.Clicked() {
						a.chatList.FollowEnd = true
						a.chatList.ScrollToEnd()
						a.chatSeen = a.chatNext
					}
				})
			}
			a.connectionOverlay(c, opacity)
		})
	})
}

func (a *app) chatEmptyView(c *ui.Context) {
	p := colors(c.Theme())
	ui.Column(c).Fill().Center().Padding(25).Gap(10).Children(func() {
		ui.Box(c).Size(42, 42).Radius(14).Background(p.hover).Center().Children(func() {
			ui.Icon(c, iconChat).Size(21, 21).TextColor(p.faint)
		})
		title := "等一条新弹幕"
		if a.chatOnlySC {
			title = "还没有醒目留言"
		}
		ui.Text(c, title).FontSize(12).FontWeight(500).TextColor(p.muted)
	})
}

func (a *app) chatBubble(c *ui.Context, msg platforms.ChatMessage) {
	p := colors(c.Theme())
	prefs := library.Preferences{ChatSize: 13, ChatBadges: true}
	if a.library != nil {
		prefs = a.library.Preferences
	}
	font := float32(prefs.ChatSize)
	if font < 11 {
		font = 13
	}
	kind := msg.Kind == "superchat"
	bubble := p.soft
	edge := p.line
	if kind {
		bubble = p.accent.Alpha(.08)
		edge = p.accent.Alpha(.2)
	}
	menu := func(m *ui.Menu) {
		if m.Item("复制消息").Chosen() {
			mygo.Clipboard.WriteText(msg.User + "：" + msg.Text)
			a.toast = "已复制消息"
		}
		if m.Item("复制昵称").Chosen() {
			mygo.Clipboard.WriteText(msg.User)
			a.toast = "已复制昵称"
		}
		m.Separator()
		if m.Item("在本机屏蔽该用户").Chosen() {
			a.blockChatUser(msg)
		}
		if m.Item("屏蔽关键词…").Chosen() {
			a.filterDraft = msg.Text
			a.filterWindow()
		}
	}
	if prefs.ChatCompact && !kind {
		box := ui.Box(c.Key(msg.ID)).FillWidth().Padding(4, 14).Label(msg.User + "：" + msg.Text).ContextMenu(menu)
		box.Children(func() { a.chatBody(c, msg, font, true).MaxWidth(258).ContextMenu(menu) })
		if box.DoubleClicked() {
			mygo.Clipboard.WriteText(msg.User + "：" + msg.Text)
			a.toast = "已复制消息"
		}
		return
	}
	ui.Row(c.Key(msg.ID)).Padding(4, 13).AlignItems(ui.Start).Children(func() {
		box := ui.Column(c).MaxWidth(258).MinWidth(104).Padding(9, 11).Gap(5).Radius(11, 11, 11, 4).Background(bubble).Border(.5, edge).Label(msg.User + "：" + msg.Text).ContextMenu(menu)
		box.Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				ui.Text(c, msg.User).FontSize(10.5).FontWeight(600).TextColor(p.muted).SingleLine().MinWidth(0).MaxWidth(155).Tooltip(msg.User)
				if prefs.ChatBadges && msg.Medal != "" {
					ui.Textf(c, "%s %d", msg.Medal, msg.MedalLevel).FontSize(8.5).FontWeight(500).TextColor(p.accent).Background(p.accent.Alpha(.075)).Radius(3).Padding(2, 3).SingleLine()
				}
				if prefs.ChatTime && !msg.Received.IsZero() {
					ui.Text(c, msg.Received.Format("15:04")).FontSize(9).TextColor(p.faint).MarginStart(4)
				}
			})
			if kind {
				ui.Textf(c, "醒目留言  ¥%g", msg.Price).FontSize(10).FontWeight(600).TextColor(p.accent)
			}
			a.chatBody(c, msg, font).ContextMenu(menu)
		})
		if box.DoubleClicked() {
			mygo.Clipboard.WriteText(msg.User + "：" + msg.Text)
			a.toast = "已复制消息"
		}
	})
}

func (a *app) chatBody(c *ui.Context, msg platforms.ChatMessage, size float32, compact ...bool) ui.Element {
	p := colors(c.Theme())
	remaining := msg.Text
	var spans []ui.Span
	objects := map[int]*ui.Bitmap{}
	runes := 0
	if len(compact) > 0 && compact[0] {
		prefix := msg.User + "："
		if a.library != nil && a.library.Preferences.ChatTime && !msg.Received.IsZero() {
			prefix = msg.Received.Format("15:04") + " " + prefix
		}
		spans = append(spans, ui.Span{Text: prefix, Size: size - 1, Weight: 600, Color: p.muted})
		runes = utf8.RuneCountInString(prefix)
		if a.library != nil && a.library.Preferences.ChatBadges && msg.Medal != "" {
			badge := fmt.Sprintf("[%s %d] ", msg.Medal, msg.MedalLevel)
			spans = append(spans, ui.Span{Text: badge, Size: size - 2, Color: p.accent})
			runes += utf8.RuneCountInString(badge)
		}
	}
	for remaining != "" {
		pos := -1
		var chosen platforms.ChatEmote
		var bitmap *ui.Bitmap
		for _, emote := range msg.Emotes {
			if emote.Text == "" {
				continue
			}
			index := strings.Index(remaining, emote.Text)
			if index >= 0 && (pos < 0 || index < pos) {
				if img := a.cover(emote.URL); img != nil {
					pos, chosen, bitmap = index, emote, img
				}
			}
		}
		if pos < 0 {
			spans = append(spans, ui.Span{Text: remaining})
			break
		}
		if pos > 0 {
			spans = append(spans, ui.Span{Text: remaining[:pos]})
			runes += utf8.RuneCountInString(remaining[:pos])
		}
		spans = append(spans, ui.Span{Text: "\uFFFC", ObjectWidth: size * 1.65, ObjectHeight: size * 1.5, ObjectDescent: 3})
		objects[runes] = bitmap
		runes++
		remaining = remaining[pos+len(chosen.Text):]
	}
	return ui.RichText(c, spans...).FontSize(size).LineHeight(1.45).MaxWidth(234).TextColor(p.text).Selectable().Tooltip(msg.Text).DrawObjects(func(p *ui.Painter, at int, r ui.Rect) {
		if img := objects[at]; img != nil {
			p.Image(img, r, ui.Contain)
		}
	})
}

func (a *app) clearChat() {
	a.chatMessages = nil
	a.chatRecent = nil
	a.chatSeen = a.chatNext
	a.chatList = ui.ListState{FollowEnd: true}
	a.clearFlyingChat()
}
