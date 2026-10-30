//go:build darwin || windows

package app

import (
	"context"
	"fmt"
	"time"
)

func chatRetryDelay(attempt int) time.Duration {
	return []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}[min(attempt, 4)]
}

func reconnectChat(ctx context.Context, listen func(func()) error, status func(string), delay func(int) time.Duration) {
	attempt := 0
	for ctx.Err() == nil {
		err := listen(func() { attempt = 0 })
		if ctx.Err() != nil {
			return
		}
		reason := "弹幕连接已中断"
		if err != nil {
			reason = err.Error()
		}
		if attempt >= 8 {
			status(reason + "，自动重连未成功，请手动重连")
			return
		}
		wait := delay(attempt)
		attempt++
		status(fmt.Sprintf("%s，%g 秒后重连", reason, wait.Seconds()))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
