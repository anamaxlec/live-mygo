//go:build darwin || windows

package app

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"
)

// runNativeVideo exercises the system decoder without loading a room or accessing account data.
func runNativeVideo(source string, opts options) error {
	isURL := strings.Contains(source, "://")
	if isURL {
		u, err := url.Parse(source)
		if err != nil || u.Hostname() == "" || u.Scheme != "https" && u.Scheme != "http" {
			return errors.New("原生视频验证需要本地文件或 HTTP(S) 视频地址")
		}
	} else if info, err := os.Stat(source); err != nil || !info.Mode().IsRegular() {
		return errors.New("原生视频验证需要可读取的本地视频文件")
	}

	var videoErr, reportErr error
	mygo.App.SetName("Live MyGo")
	mygo.App.WhenReady(func() {
		started := time.Now()
		var p *ui.VideoPlayer
		if isURL {
			p, videoErr = video.OpenURL(source)
		} else {
			p, videoErr = video.Open(source)
		}
		if videoErr != nil {
			videoErr = errors.New("MyGo 原生视频初始化失败")
			mygo.App.Quit()
			return
		}
		if opts.mute {
			// SetMuted treats the clip as decorative and can stop it under Reduce Motion.
			p.SetVolume(0)
		}
		var firstFrame time.Duration
		toggle := func() {
			if p.Playing() {
				p.Pause()
			} else {
				p.Play()
			}
		}
		win := mygo.NewWindow(mygo.WindowOptions{
			Title: "Live MyGo · MyGo 原生视频验证", Width: 960, Height: 580,
			Content: ui.View(func(c *ui.Context) {
				c.OnShortcut(0, ui.KeySpace, toggle)
				c.Root().Background(ui.RGB(0, 0, 0))
				ui.Column(c).Fill().Children(func() {
					ui.Box(c).Grow(1).FillWidth().Center().Children(func() {
						ui.Video(c, p).Fill().Fit(ui.Contain).Label("原生视频")
						switch {
						case p.Err() != nil:
							ui.Text(c, "系统播放器无法播放此视频").Absolute().TextColor(ui.RGB(255, 255, 255))
						case !p.Ready():
							// AVFoundation can have a frame ready before MyGo signals a state change.
							c.After(33 * time.Millisecond)
							ui.Spinner(c).Absolute()
						case p.Buffering():
							ui.Spinner(c).Absolute()
						}
						if firstFrame == 0 && p.Ready() {
							firstFrame = time.Since(started)
						}
					})
					ui.Row(c).Height(44).PaddingX(12).AlignItems(ui.Center).Gap(8).Children(func() {
						label := "播放"
						if p.Playing() {
							label = "暂停"
						}
						if ui.Button(c, label).Disabled(p.Err() != nil).Clicked() {
							toggle()
						}
						label = "静音"
						if p.Volume() == 0 {
							label = "取消静音"
						}
						if ui.Button(c, label).Clicked() {
							if p.Volume() == 0 {
								p.SetVolume(1)
							} else {
								p.SetVolume(0)
							}
						}
					})
				})
			}),
		})

		mygo.App.Focus()
		win.Focus()

		var timer *time.Timer
		if opts.duration > 0 {
			timer = time.AfterFunc(opts.duration, func() { win.Update(win.Close) })
		}
		win.OnClosed(func() {
			if timer != nil {
				timer.Stop()
			}
			defer p.Close()
			if p.Err() != nil {
				videoErr = errors.New("系统播放器无法播放此视频")
			} else if firstFrame == 0 {
				videoErr = errors.New("原生视频未取得首帧")
			}
			if opts.report == "" {
				return
			}
			w, h := p.Size()
			report := struct {
				FirstFrameMS int64 `json:"first_frame_ready_ms"`
				PositionMS   int64 `json:"position_ms"`
				Width        int   `json:"width"`
				Height       int   `json:"height"`
				Failed       bool  `json:"failed"`
			}{firstFrame.Milliseconds(), p.Position().Milliseconds(), w, h, videoErr != nil}
			data, err := json.MarshalIndent(report, "", "  ")
			if err == nil {
				err = os.MkdirAll(filepath.Dir(opts.report), 0755)
			}
			if err == nil {
				err = os.WriteFile(opts.report, append(data, '\n'), 0600)
			}
			reportErr = err
		})
	})
	return errors.Join(mygo.App.Run(), videoErr, reportErr)
}
