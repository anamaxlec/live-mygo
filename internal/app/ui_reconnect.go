//go:build darwin || windows

package app

import (
	"fmt"
	"time"
)

// 现场验证发现直播源会自行结束；只对正在观看的直播重新取流，本地文件不重连。
func (a *app) scheduleReconnect() {
	if a.stream == nil || a.page != "player" || a.closing || a.busy || a.reconnectTimer != nil || a.reconnectAttempts >= 3 {
		return
	}
	a.reconnectAttempts++
	gen := a.generation
	delay := []time.Duration{500 * time.Millisecond, 2 * time.Second, 5 * time.Second}[a.reconnectAttempts-1]
	a.status = fmt.Sprintf("直播连接中断，正在重连（%d/3）", a.reconnectAttempts)
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
				a.resolve(a.stream.Room.Platform, a.stream.Room.ID, a.stream.QualityID, a.stream.SourceID)
			}
		})
	})
}
