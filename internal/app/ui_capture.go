//go:build darwin || windows

package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"live-mygo/internal/player"
)

func captureFilename(title, extension string, now time.Time) string {
	name := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, title))
	if len(name) > 40 {
		name = name[:40]
	}
	label := strings.Trim(strings.TrimSpace(string(name)), ". ")
	if label == "" {
		label = "LiveMyGo"
	}
	return label + "-" + now.Format("20060102-150405") + extension
}

func (a *app) captureMenu(m *ui.Menu) {
	if m.Item("保存当前画面").Disabled(!a.shown || a.audioOnly() || a.captureBusy).Chosen() {
		a.saveCapture(false)
	}
	if a.recordingPath != "" {
		if m.Item("停止录制").Chosen() {
			a.stopRecording()
		}
	} else if m.Item("录制 30 秒片段").Disabled(!a.shown || a.stats.Paused || a.captureBusy).Chosen() {
		a.saveCapture(true)
	}
	if a.lastCapturePath != "" && m.Item("在文件夹中显示上次保存").Chosen() {
		mygo.Shell.ShowItemInFolder(a.lastCapturePath)
	}
}

func (a *app) saveCapture(record bool) {
	if a.win == nil || a.player == nil || !a.shown || a.captureBusy || record && a.recordingPath != "" {
		return
	}
	a.captureBusy = true
	generation, title := a.generation, a.title
	extension, label, filter := ".png", "保存当前画面", "PNG 图片"
	if record {
		extension, label, filter = ".mkv", "保存 30 秒片段", "Matroska 视频"
	}
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.win, Title: label, DefaultPath: captureFilename(title, extension, time.Now()), Filters: []mygo.FileFilter{{Name: filter, Extensions: []string{strings.TrimPrefix(extension, ".")}}}})
		a.win.Update(func() {
			if a.closing {
				return
			}
			if err != nil || path == "" {
				a.captureBusy = false
				if err != nil {
					a.toast = "无法打开保存窗口"
				}
				return
			}
			if generation != a.generation || !a.shown || record && a.stats.Paused || !record && a.audioOnly() {
				a.captureBusy = false
				a.toast = "播放状态已变化，请重新保存"
				return
			}
			if filepath.Ext(path) == "" {
				path += extension
			}
			args := []string{"screenshot-to-file", path, "video"}
			if record {
				args = []string{"set", "stream-record", path}
			}
			a.doPlayer(func(p *player.Player) error { return p.Command(args...) }, func(err error) {
				a.captureBusy = false
				if err != nil {
					a.toast = label + "失败：" + err.Error()
					return
				}
				if !record {
					a.lastCapturePath, a.toast = path, "画面已保存"
					return
				}
				// 保存命令可能在切房间前已排入队列；此时收尾文件，不把录制延续到下一房间。
				a.recordingPath = path
				if generation != a.generation || !a.shown || a.stats.Paused {
					a.stopRecording()
					return
				}
				a.recordingUntil = time.Now().Add(30 * time.Second)
				deadline := a.recordingUntil
				a.recordingTimer = time.AfterFunc(30*time.Second, func() {
					a.win.Update(func() {
						if !a.closing && a.recordingPath == path && a.recordingUntil.Equal(deadline) {
							a.stopRecording()
						}
					})
				})
				a.toast = "开始录制，30 秒后自动保存"
			})
		})
	}()
}

func (a *app) stopRecording() {
	if a.recordingTimer != nil {
		a.recordingTimer.Stop()
		a.recordingTimer = nil
	}
	path := a.recordingPath
	if path == "" {
		return
	}
	a.recordingPath, a.recordingUntil = "", time.Time{}
	a.doPlayer(func(p *player.Player) error { return p.Command("set", "stream-record", "") }, func(err error) {
		if err != nil {
			a.toast = "片段未完整保存：" + err.Error()
			return
		}
		a.lastCapturePath, a.toast = path, "片段已保存"
	})
}

func (a *app) recordingLabel() string {
	seconds := max(0, int(time.Until(a.recordingUntil).Seconds()))
	return fmt.Sprintf("录制中 · %02d 秒", seconds)
}
