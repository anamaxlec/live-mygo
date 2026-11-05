//go:build darwin || windows

package app

import (
	"fmt"
	"live-mygo/internal/platforms"
	"time"
)

// 先刷新当前线路的短期地址，再尝试平台已经返回的其他线路。
func reconnectSourcePlan(stream *platforms.Stream) []string {
	if stream == nil {
		return nil
	}
	plan := []string{stream.SourceID}
	for _, source := range stream.Sources {
		duplicate := false
		for _, id := range plan {
			if id == source.ID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			plan = append(plan, source.ID)
		}
	}
	return plan
}

// 现场验证发现直播源会自行结束；只对正在观看的直播重新取流，本地文件不重连。
func (a *app) scheduleReconnect() {
	if a.stream == nil || a.page != "player" || a.closing || a.busy || a.reconnectTimer != nil || a.reconnectAttempts >= 3 {
		return
	}
	if a.reconnectAttempts == 0 {
		a.reconnectSources = reconnectSourcePlan(a.stream)
	}
	a.reconnectAttempts++
	route := a.stream.SourceID
	if len(a.reconnectSources) > 0 {
		route = a.reconnectSources[(a.reconnectAttempts-1)%len(a.reconnectSources)]
	}
	gen := a.generation
	delay := []time.Duration{500 * time.Millisecond, 2 * time.Second, 5 * time.Second}[a.reconnectAttempts-1]
	a.status = fmt.Sprintf("直播连接中断，正在重连（%d/3）", a.reconnectAttempts)
	if route != a.stream.SourceID {
		a.status = fmt.Sprintf("正在尝试其他播放线路（%d/3）", a.reconnectAttempts)
	}
	a.busy = true
	a.revealControls()
	a.reconnectTimer = time.AfterFunc(delay, func() {
		a.win.Update(func() {
			if a.closing || a.generation != gen {
				return
			}
			a.reconnectTimer = nil
			a.busy = false
			if a.stream != nil {
				a.resolve(a.stream.Room.Platform, a.stream.Room.ID, a.stream.QualityID, route)
			}
		})
	})
}
