//go:build darwin || windows

package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"live-mygo/internal/player"
)

type diagnosticFault struct {
	At     time.Time `json:"at"`
	Stage  string    `json:"stage"`
	Reason string    `json:"reason"`
}

func (a *app) openCrashReports() {
	if runtime.GOOS != "darwin" {
		return
	}
	home, err := os.UserHomeDir()
	if err == nil {
		err = mygo.Shell.OpenPath(filepath.Join(home, "Library", "Logs", "DiagnosticReports"))
	}
	if err != nil {
		a.settings.message = "暂时无法打开崩溃报告目录"
	}
}

// 错误原文可能带播放地址、签名或请求头。导出只使用固定的错误分类。
func failureCategory(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "deadline"), strings.Contains(lower, "超时"):
		return "timeout"
	case strings.Contains(lower, "tls"), strings.Contains(lower, "certificate"), strings.Contains(lower, "证书"), strings.Contains(lower, "加密"):
		return "tls"
	case strings.Contains(lower, "decode"), strings.Contains(lower, "解码"), strings.Contains(lower, "videotoolbox"):
		return "decoder"
	case strings.Contains(lower, "audio"), strings.Contains(lower, "音频"):
		return "audio"
	case strings.Contains(lower, "eof"), strings.Contains(lower, "结束"):
		return "stream-ended"
	case strings.Contains(lower, "未开播"):
		return "offline"
	default:
		return "connection"
	}
}

func (a *app) recordFailure(stage, message string) {
	fault := diagnosticFault{At: time.Now(), Stage: stage, Reason: failureCategory(message)}
	if n := len(a.recentFaults); n > 0 {
		previous := a.recentFaults[n-1]
		if previous.Stage == fault.Stage && previous.Reason == fault.Reason && fault.At.Sub(previous.At) < 5*time.Second {
			return
		}
	}
	a.recentFaults = append(a.recentFaults, fault)
	if len(a.recentFaults) > 12 {
		a.recentFaults = a.recentFaults[len(a.recentFaults)-12:]
	}
}

func (a *app) diagnosticData() ([]byte, error) {
	stats := a.stats
	stats.Error = ""
	metrics := player.RenderMetrics{}
	if a.surface != nil {
		metrics = a.surface.Metrics()
	}
	state := "stopped"
	if a.busy {
		state = "connecting"
	} else if a.shown {
		state = "playing"
		if stats.Paused {
			state = "paused"
		}
	} else if len(a.recentFaults) > 0 {
		state = "failed"
	}
	report := struct {
		Created      time.Time            `json:"created"`
		Version      string               `json:"version"`
		OS           string               `json:"os"`
		Architecture string               `json:"architecture"`
		State        string               `json:"state"`
		Decoder      player.Stats         `json:"playback"`
		Rendering    player.RenderMetrics `json:"rendering"`
		FirstFrameMS int64                `json:"first_frame_ms"`
		Retries      int                  `json:"recovery_attempts"`
		Faults       []diagnosticFault    `json:"recent_failures,omitempty"`
	}{time.Now(), appVersion, runtime.GOOS, runtime.GOARCH, state, stats, metrics, a.firstFrameMS, a.reconnectAttempts, a.recentFaults}
	return json.MarshalIndent(report, "", "  ")
}

func (a *app) exportDiagnostics() {
	if a.diagnosticsExporting || a.win == nil {
		return
	}
	data, err := a.diagnosticData()
	if err != nil {
		a.toast = "生成诊断报告失败"
		return
	}
	parent := a.win
	if a.accountWin != nil && !a.accountWin.IsDestroyed() {
		parent = a.accountWin
	}
	a.diagnosticsExporting = true
	go func() {
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: parent, Title: "导出诊断报告", DefaultPath: "LiveMyGo-diagnostics-" + time.Now().Format("20060102-150405") + ".json", Filters: []mygo.FileFilter{{Name: "JSON", Extensions: []string{"json"}}}})
		if err == nil && path != "" {
			err = os.WriteFile(path, append(data, '\n'), 0600)
		}
		a.win.Update(func() {
			a.diagnosticsExporting = false
			if a.closing {
				return
			}
			if err != nil {
				a.toast = "导出诊断报告失败"
			} else if path != "" {
				a.toast = "已导出诊断报告"
			}
			a.settings.message = a.toast
			if a.accountWin != nil && !a.accountWin.IsDestroyed() {
				a.accountWin.Invalidate()
			}
		})
	}()
}
