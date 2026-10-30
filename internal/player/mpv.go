//go:build darwin || windows

package player

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type bindings struct {
	create      func() uintptr
	initialize  func(uintptr) int32
	option      func(uintptr, string, string) int32
	command     func(uintptr, unsafe.Pointer) int32
	commandNode func(uintptr, unsafe.Pointer, unsafe.Pointer) int32
	property    func(uintptr, string) unsafe.Pointer
	free        func(unsafe.Pointer)
	destroy     func(uintptr)
	waitEvent   func(uintptr, float64) unsafe.Pointer
	errorString func(int32) string
}

type Player struct {
	mu       sync.Mutex
	api      bindings
	handle   uintptr
	lastErr  string
	renderer *renderer
	barrage  barrageState
}
type Stats struct {
	Position   float64 `json:"position_seconds"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FPS        float64 `json:"fps"`
	Hardware   string  `json:"hardware_decoder"`
	VideoCodec string  `json:"video_codec"`
	AudioCodec string  `json:"audio_codec"`
	AVSync     float64 `json:"av_sync_seconds"`
	Dropped    int     `json:"dropped_frames"`
	Visible    bool    `json:"video_surface_visible"`
	Paused     bool    `json:"paused"`
	Muted      bool    `json:"muted"`
	Idle       bool    `json:"idle"`
	Error      string  `json:"error,omitempty"`
}

func New(surface *Surface) (*Player, error) {
	lib, err := loadLibrary()
	if err != nil {
		return nil, err
	}
	p := &Player{}
	for _, entry := range []struct {
		name string
		fn   any
	}{
		{"mpv_create", &p.api.create}, {"mpv_initialize", &p.api.initialize}, {"mpv_set_option_string", &p.api.option},
		{"mpv_command", &p.api.command}, {"mpv_get_property_string", &p.api.property}, {"mpv_free", &p.api.free},
		{"mpv_command_node", &p.api.commandNode},
		{"mpv_terminate_destroy", &p.api.destroy}, {"mpv_wait_event", &p.api.waitEvent}, {"mpv_error_string", &p.api.errorString},
	} {
		sym, err := librarySymbol(lib, entry.name)
		if err != nil {
			return nil, fmt.Errorf("libmpv 缺少 %s", entry.name)
		}
		purego.RegisterFunc(entry.fn, sym)
	}
	p.handle = p.api.create()
	if p.handle == 0 {
		return nil, errors.New("无法创建 libmpv 播放器")
	}
	opts := [][2]string{{"config", "no"}, {"terminal", "no"}, {"msg-level", "all=no"}, {"input-default-bindings", "no"}, {"input-media-keys", "no"}, {"osc", "no"}, {"hwdec", "auto-safe"}, {"volume", "60"}, {"cache", "yes"}, {"cache-secs", "2"}, {"demuxer-readahead-secs", "2"}, {"demuxer-max-bytes", "32MiB"}}
	if logPath := os.Getenv("LIVE_MYGO_MPV_LOG"); logPath != "" {
		opts = append(opts, [2]string{"log-file", logPath}, [2]string{"msg-level", "all=v"})
	}
	if os.Getenv("LIVE_MYGO_QA_LOOP") != "" {
		opts = append(opts, [2]string{"loop-file", "inf"})
	}
	opts = append(opts, [2]string{"tls-verify", "yes"})
	if executable, err := os.Executable(); err == nil {
		for _, certificate := range []string{filepath.Join(filepath.Dir(executable), "../Resources/cacert.pem"), filepath.Join(filepath.Dir(executable), "cacert.pem")} {
			if _, err = os.Stat(certificate); err == nil {
				opts = append(opts, [2]string{"tls-ca-file", certificate})
				break
			}
		}
	}
	if surface != nil {
		opts = append(opts, surfaceOptions(surface)...)
	} else {
		opts = append(opts, [2]string{"vo", "null"}, [2]string{"ao", "null"})
	}
	for _, kv := range opts {
		if rc := p.api.option(p.handle, kv[0], kv[1]); rc < 0 {
			p.api.destroy(p.handle)
			return nil, fmt.Errorf("libmpv 配置 %s 失败：%s", kv[0], p.api.errorString(rc))
		}
	}
	if rc := p.api.initialize(p.handle); rc < 0 {
		p.api.destroy(p.handle)
		return nil, fmt.Errorf("libmpv 初始化失败：%s", p.api.errorString(rc))
	}
	if surface != nil {
		if err := p.initRenderer(lib, surface); err != nil {
			p.api.destroy(p.handle)
			return nil, err
		}
	}
	return p, nil
}

func (p *Player) Load(target string, headers map[string]string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return errors.New("播放器已关闭")
	}
	// 每次切源先清空上一平台的请求头，避免账号跨平台复用。
	for _, kv := range [][2]string{{"user-agent", headers["User-Agent"]}, {"referrer", headers["Referer"]}, {"http-header-fields", headerFields(headers)}} {
		if err := p.command("set", kv[0], kv[1]); err != nil {
			return err
		}
	}
	p.lastErr = ""
	p.clearBarrage()
	if err := p.command("loadfile", target, "replace"); err != nil {
		return err
	}
	return p.command("set", "pause", "no")
}

func headerFields(headers map[string]string) string {
	var names []string
	for k := range headers {
		if k != "User-Agent" && k != "Referer" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	var values []string
	for _, k := range names {
		v := k + ": " + headers[k]
		values = append(values, fmt.Sprintf("%%%d%%%s", len(v), v))
	}
	return strings.Join(values, ",")
}

func (p *Player) Command(args ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return errors.New("播放器已关闭")
	}
	return p.command(args...)
}
func (p *Player) command(args ...string) error {
	bufs := make([][]byte, len(args))
	ptrs := make([]*byte, len(args)+1)
	for i, s := range args {
		if strings.IndexByte(s, 0) >= 0 {
			return errors.New("播放器参数包含无效字符")
		}
		bufs[i] = append([]byte(s), 0)
		ptrs[i] = &bufs[i][0]
	}
	rc := p.api.command(p.handle, unsafe.Pointer(&ptrs[0]))
	runtime.KeepAlive(bufs)
	runtime.KeepAlive(ptrs)
	if rc < 0 {
		return fmt.Errorf("播放器命令失败：%s", p.api.errorString(rc))
	}
	return nil
}

func (p *Player) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle == 0 {
		return Stats{Idle: true}
	}
	type event struct {
		ID       int32
		Error    int32
		Userdata uint64
		Data     unsafe.Pointer
	}
	for i := 0; i < 100; i++ {
		addr := p.api.waitEvent(p.handle, 0)
		if addr == nil {
			break
		}
		e := *(*event)(addr)
		if e.ID == 0 {
			break
		}
		if e.ID == 7 && e.Data != nil {
			end := (*[2]int32)(e.Data)
			if end[1] < 0 {
				p.lastErr = "播放失败：" + p.api.errorString(end[1])
			}
		}
	}
	get := func(name string) string {
		ptr := p.api.property(p.handle, name)
		if ptr == nil {
			return ""
		}
		defer p.api.free(ptr)
		return cstring(ptr)
	}
	num := func(name string) float64 { v, _ := strconv.ParseFloat(get(name), 64); return v }
	stats := Stats{Position: num("time-pos"), Width: int(num("video-params/w")), Height: int(num("video-params/h")), FPS: num("container-fps"), Hardware: get("hwdec-current"), VideoCodec: get("video-codec"), AudioCodec: get("audio-codec-name"), AVSync: num("avsync"), Dropped: int(num("frame-drop-count")), Paused: get("pause") == "yes", Muted: get("mute") == "yes", Idle: get("idle-active") == "yes", Error: p.lastErr}
	if p.renderer != nil {
		stats.Visible = p.renderer.surface.visible.Load()
		if stats.Error == "" {
			stats.Error = p.renderer.surface.RenderError()
		}
	}
	if stats.Width > 0 && stats.Height > 0 {
		p.barrage.VideoAspect = float64(stats.Width) / float64(stats.Height)
	}
	return stats
}

func (p *Player) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.handle != 0 {
		if p.renderer != nil {
			p.renderer.close()
			p.renderer = nil
		}
		p.api.destroy(p.handle)
		p.handle = 0
	}
}
func cstring(ptr unsafe.Pointer) string {
	b := make([]byte, 0, 64)
	for i := uintptr(0); i < 65536; i++ {
		v := *(*byte)(unsafe.Add(ptr, i))
		if v == 0 {
			break
		}
		b = append(b, v)
	}
	return string(b)
}
