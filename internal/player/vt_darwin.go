//go:build darwin && cgo

package player

/*
#cgo CFLAGS: -I/opt/homebrew/include -fobjc-arc
#cgo LDFLAGS: -L/opt/homebrew/lib -lavformat -lavcodec -lavutil -framework VideoToolbox -framework CoreMedia -framework CoreVideo -framework CoreImage -framework ImageIO -framework AVFoundation -framework AudioToolbox -framework Metal -framework QuartzCore -framework AppKit -framework Foundation
#include "vt_darwin.h"
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

const vtBackendAvailable = true

type VTSurface struct {
	keepAspect   atomic.Bool
	fillVideo    atomic.Bool
	view         unsafe.Pointer
	visible      atomic.Bool
	rendered     atomic.Uint64
	resizes      atomic.Uint64
	maxResizeNS  atomic.Int64
	width        atomic.Int64
	height       atomic.Int64
	cornerRadius float64
	cornerRight  bool
}

func newVTSurface(window uintptr) (VideoSurface, error) {
	view := C.vt_view_create(C.uintptr_t(window))
	if view == nil {
		return nil, errors.New("无法创建 CAMetalLayer 视频视图")
	}
	s := &VTSurface{view: view}
	s.keepAspect.Store(true)
	return s, nil
}

func (s *VTSurface) Show(show bool) {
	var visible C.int
	C.vt_view_show(s.view, boolInt(show), &visible)
	s.visible.Store(visible != 0)
}

func (s *VTSurface) SetBounds(x, y, w, h, windowHeight float64) bool {
	started := time.Now()
	var width, height, changed C.int
	if C.vt_view_set_bounds(s.view, C.double(x), C.double(y), C.double(w), C.double(h), C.double(windowHeight), &width, &height, &changed) == 0 {
		return false
	}
	s.width.Store(int64(width))
	s.height.Store(int64(height))
	if changed == 0 {
		return true
	}
	s.resizes.Add(1)
	elapsed := time.Since(started).Nanoseconds()
	for old := s.maxResizeNS.Load(); elapsed > old; old = s.maxResizeNS.Load() {
		if s.maxResizeNS.CompareAndSwap(old, elapsed) {
			break
		}
	}
	return true
}

func (s *VTSurface) SetCorners(radius float64, right bool) {
	if radius == s.cornerRadius && right == s.cornerRight {
		return
	}
	s.cornerRadius = radius
	s.cornerRight = right
	C.vt_view_set_corners(s.view, C.double(radius), boolInt(right))
}

func (s *VTSurface) SetWindowButtonsVisible(visible bool) {
	C.vt_view_set_buttons(s.view, boolInt(visible))
}

func (s *VTSurface) Metrics() RenderMetrics {
	rendered := s.rendered.Load()
	return RenderMetrics{
		Resizes:         s.resizes.Load(),
		MaxResizeMS:     float64(s.maxResizeNS.Load()) / 1e6,
		RenderedFrames:  rendered,
		UpdateCallbacks: rendered,
		Backend:         "videotoolbox-metal",
	}
}

func (s *VTSurface) RenderError() string {
	message := C.vt_view_error(s.view)
	if message == nil {
		return ""
	}
	return C.GoString(message)
}

func (s *VTSurface) SetFrameCallback(func()) {}
func (s *VTSurface) Close()                  { C.vt_view_close(s.view); s.view = nil }

func (s *VTSurface) applyFit() {
	C.vt_view_set_fit(s.view, boolInt(s.keepAspect.Load()), boolInt(s.fillVideo.Load()))
}

func (s *VTSurface) present(frame unsafe.Pointer) bool {
	if frame == nil {
		return false
	}
	if C.vt_view_present(s.view, frame) == 0 {
		return false
	}
	s.rendered.Add(1)
	return true
}

type vtPlayer struct {
	target  string
	headers map[string]string
	surface *VTSurface

	sessionMu    sync.Mutex
	closed       bool
	mu           sync.Mutex
	core         *C.VTCore
	audio        unsafe.Pointer
	audioRate    int
	audioStarted bool
	audioFailed  bool
	audioWait    time.Time
	volume       float64
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	paused       bool
	audioOnly    bool
	muted        bool
	idle         bool
	errMsg       string
	originPts    float64
	originSet    bool
	basePts      float64
	started      time.Time
	droppedLocal atomic.Int64
	lastPts      float64
	position     float64
	width        int
	height       int
	fps          float64
	codec        string
	dropped      int
	presented    atomic.Int64
	taken        atomic.Int64
	presentOK    atomic.Int64
	presentFail  atomic.Int64
	lastFrameAt  time.Time
}

func newVTPlayer(surface VideoSurface) (*Player, error) {
	vt, ok := surface.(*VTSurface)
	if !ok {
		return nil, errors.New("VideoToolbox 后端需要 CAMetalLayer 视频表面")
	}
	return &Player{vt: &vtPlayer{surface: vt, volume: 60}}, nil
}

func (p *vtPlayer) Load(target string, headers map[string]string) error {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	return p.load(target, headers)
}

func (p *vtPlayer) load(target string, headers map[string]string) error {
	if p.closed {
		return errors.New("播放器已关闭")
	}
	p.stopSession()
	if strings.ContainsAny(target, "\x00\r\n") {
		return errors.New("播放地址包含无效字符")
	}
	for key, value := range headers {
		if strings.ContainsAny(key+value, "\x00\r\n") {
			return errors.New("播放请求头包含无效字符")
		}
	}

	url := C.CString(target)
	userAgent := C.CString(vtHeader(headers, "User-Agent"))
	referer := C.CString(vtHeader(headers, "Referer"))
	headerBlock := C.CString(vtHeaderBlock(headers))
	caFile := C.CString(vtCAFile())
	var errBuf [512]C.char
	core := C.vt_core_open(url, userAgent, referer, headerBlock, caFile, &errBuf[0], C.int(len(errBuf)))
	C.free(unsafe.Pointer(url))
	C.free(unsafe.Pointer(userAgent))
	C.free(unsafe.Pointer(referer))
	C.free(unsafe.Pointer(headerBlock))
	C.free(unsafe.Pointer(caFile))
	if core == nil {
		return errors.New(vtErrorMessage(errBuf[:]))
	}

	codec := C.vt_core_video_codec(core)
	if codec != C.vt_codec_h264() && codec != C.vt_codec_hevc() {
		C.vt_core_close(core)
		return fmt.Errorf("原生硬解不支持该视频编码（%d），请在播放设置中切换到 mpv", int(codec))
	}
	if C.vt_core_start_decoder(core, &errBuf[0], C.int(len(errBuf))) == 0 {
		// decoder ready
	} else {
		message := vtErrorMessage(errBuf[:])
		C.vt_core_close(core)
		return errors.New(message)
	}

	var audio unsafe.Pointer
	audioRate := 0
	if C.vt_core_audio_codec(core) == C.vt_codec_aac() {
		var extraSize C.int
		extra := C.vt_core_audio_extradata(core, &extraSize)
		rate := int(C.vt_core_audio_sample_rate(core))
		channels := int(C.vt_core_audio_channels(core))
		if extra != nil && extraSize > 0 && rate > 0 && channels > 0 {
			var audioErr [512]C.char
			audio = C.vt_audio_create(C.int(rate), C.int(channels), extra, extraSize, &audioErr[0], C.int(len(audioErr)))
			if audio == nil {
				fmt.Fprintf(os.Stderr, "VT 音频初始化失败：%s\n", vtErrorMessage(audioErr[:]))
			} else {
				audioRate = rate
			}
		}
	}

	codecName := "h264"
	if codec == C.vt_codec_hevc() {
		codecName = "hevc"
	}
	if C.vt_core_audio_codec(core) != 0 && audio == nil {
		C.vt_core_close(core)
		return errors.New("原生硬解无法播放此音轨，请在播放设置中切换到 mpv")
	}
	p.mu.Lock()
	onlyAudio := p.audioOnly
	p.mu.Unlock()
	if onlyAudio && audio == nil {
		C.vt_core_close(core)
		return errors.New("这条线路没有可播放的音轨")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.core = core
	p.target, p.headers = target, maps.Clone(headers)
	p.audio = audio
	p.audioRate = audioRate
	p.audioStarted = false
	p.audioFailed = false
	p.audioWait = time.Time{}
	p.cancel = cancel
	p.paused = false
	p.idle = false
	p.errMsg = ""
	p.originPts = 0
	p.originSet = false
	p.basePts = -1
	p.started = time.Time{}
	p.droppedLocal.Store(0)
	p.lastPts = 0
	p.position = 0
	p.width = int(C.vt_core_width(core))
	p.height = int(C.vt_core_height(core))
	p.fps = float64(C.vt_core_fps(core))
	p.codec = codecName
	p.dropped = 0
	p.lastFrameAt = time.Now()
	volume := p.volume
	muted := p.muted
	p.mu.Unlock()
	if audio != nil {
		C.vt_audio_set_volume(audio, C.double(volume/100))
		C.vt_audio_set_muted(audio, boolInt(muted))
	}
	p.presented.Store(0)
	p.taken.Store(0)
	p.presentOK.Store(0)
	p.presentFail.Store(0)

	p.wg.Add(2)
	go p.demuxLoop(ctx, core)
	go p.renderLoop(ctx, core)
	return nil
}

func (p *vtPlayer) demuxLoop(ctx context.Context, core *C.VTCore) {
	defer p.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		p.mu.Lock()
		paused := p.paused
		p.mu.Unlock()
		if paused {
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}
		var data *C.uint8_t
		var size C.int
		var pts, dts C.double
		var key, kind C.int
		var errBuf [512]C.char
		result := C.vt_core_read(core, &data, &size, &pts, &dts, &key, &kind, &errBuf[0], C.int(len(errBuf)))
		if result == 1 {
			p.mu.Lock()
			p.idle = true
			p.mu.Unlock()
			return
		}
		if result < 0 {
			if ctx.Err() == nil {
				p.setError(vtErrorMessage(errBuf[:]))
			}
			return
		}
		if data == nil || size <= 0 {
			C.vt_core_free_packet(data)
			continue
		}
		p.mu.Lock()
		audioOnly := p.audioOnly
		p.mu.Unlock()
		if kind != 1 && audioOnly {
			C.vt_core_free_packet(data)
			continue
		}
		payload := C.GoBytes(unsafe.Pointer(data), size)
		C.vt_core_free_packet(data)
		if len(payload) == 0 {
			continue
		}
		if kind == 1 {
			p.mu.Lock()
			audio := p.audio
			failed := p.audioFailed
			p.mu.Unlock()
			if audio == nil || failed {
				continue
			}
			p.enqueueAudio(ctx, audio, payload, float64(pts))
			continue
		}
		if C.vt_core_decode(core, (*C.uint8_t)(unsafe.Pointer(&payload[0])), C.int(len(payload)),
			pts, dts, key, &errBuf[0], C.int(len(errBuf))) != 0 {
			if ctx.Err() == nil {
				p.setError(vtErrorMessage(errBuf[:]))
			}
			return
		}
		for C.vt_core_queue_count(core) > 8 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Millisecond):
			}
		}
	}
}

func (p *vtPlayer) enqueueAudio(ctx context.Context, audio unsafe.Pointer, payload []byte, pts float64) {
	p.mu.Lock()
	rate := p.audioRate
	if !p.audioStarted {
		C.vt_audio_start(audio, C.double(pts))
		if p.paused {
			C.vt_audio_set_rate(audio, 0)
		}
		p.audioStarted = true
	}
	p.mu.Unlock()
	deadline := time.Now().Add(500 * time.Millisecond)
	for C.vt_audio_ready(audio) == 0 && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return
		case <-time.After(4 * time.Millisecond):
		}
	}
	duration := 1024.0 / 48000.0
	if rate > 0 {
		duration = 1024.0 / float64(rate)
	}
	var errBuf [512]C.char
	if C.vt_audio_enqueue(audio, (*C.uint8_t)(unsafe.Pointer(&payload[0])), C.int(len(payload)),
		C.double(pts), C.double(duration), &errBuf[0], C.int(len(errBuf))) != 0 {
		p.setError("音频渲染失败：" + vtErrorMessage(errBuf[:]))
		p.mu.Lock()
		p.audioFailed = true
		p.basePts = -1
		p.mu.Unlock()
		C.vt_audio_set_rate(audio, 0)
	}
}

func (p *vtPlayer) renderLoop(ctx context.Context, core *C.VTCore) {
	defer p.wg.Done()
	debug := os.Getenv("LIVE_MYGO_VT_DEBUG") == "1"
	lastLog := time.Now()
	var presentNS int64
	var lastPresent time.Time
	var maxGap, sumGap time.Duration
	var gapCount int
	wasVisible := false
	timer := time.NewTimer(0)
	defer timer.Stop()
	next := 10 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		timer.Reset(next)
		next = 10 * time.Millisecond
		p.mu.Lock()
		paused := p.paused
		idle := p.idle
		lastFrame := p.lastFrameAt
		base := p.basePts
		started := p.started
		audio := p.audio
		audioFailed := p.audioFailed
		audioOnly := p.audioOnly
		p.mu.Unlock()
		clock := math.NaN()
		if audio != nil && !audioFailed {
			clock = float64(C.vt_audio_clock(audio))
		}
		useAudio := !math.IsNaN(clock)
		if audioOnly && !paused {
			C.vt_core_flush_frames(core)
			p.mu.Lock()
			if useAudio {
				if !p.originSet {
					p.originPts = clock
					p.originSet = true
				}
				p.position = clock - p.originPts
				p.lastPts = clock
			}
			p.lastFrameAt = time.Now()
			p.mu.Unlock()
			next = 100 * time.Millisecond
			continue
		}
		if audio != nil && !audioFailed && !useAudio {
			p.mu.Lock()
			if p.audioWait.IsZero() {
				p.audioWait = time.Now()
			}
			waited := time.Since(p.audioWait)
			p.mu.Unlock()
			// 视频包可能先于首个音频包到达。先按视频时钟消费队列，
			// 避免等音频时把解封装堵在满的视频队列上，随后切到音频时钟。
			if waited > 10*time.Second {
				p.setError("音频时钟未就绪，请重新连接或切换播放内核")
				return
			}
		}
		if debug && time.Since(lastLog) > time.Second {
			lastLog = time.Now()
			var oldest, newest C.double
			var count C.int
			C.vt_core_peek_pts(core, &oldest, &newest, &count)
			target := 0.0
			if base >= 0 {
				target = base + time.Since(started).Seconds()
			}
			average := 0.0
			if gapCount > 0 {
				average = float64(sumGap.Microseconds()) / float64(gapCount) / 1000
			}
			fmt.Fprintf(os.Stderr, "vt render: taken=%d ok=%d fail=%d decoded=%d dropped=%d visible=%v queue=%d target=%.3f pts=[%.3f,%.3f] presentMS=%.1f gapAvg=%.1fms gapMax=%.1fms surfaceErr=%q\n",
				p.taken.Load(), p.presentOK.Load(), p.presentFail.Load(),
				int(C.vt_core_decoded(core)), int(C.vt_core_dropped(core)),
				p.surface.visible.Load(), int(count), target, float64(oldest), float64(newest),
				float64(presentNS)/1e6, average, float64(maxGap.Microseconds())/1000, p.surface.RenderError())
			presentNS = 0
			maxGap, sumGap, gapCount = 0, 0, 0
		}
		if paused {
			next = 100 * time.Millisecond
			continue
		}
		visible := p.surface.visible.Load()
		if visible && !wasVisible {
			p.mu.Lock()
			p.lastFrameAt = time.Now()
			lastFrame = p.lastFrameAt
			p.basePts = -1
			if audio != nil && !audioFailed {
				C.vt_audio_reset(audio)
				p.audioStarted = false
				p.audioWait = time.Time{}
			}
			p.mu.Unlock()
			base = -1
		}
		wasVisible = visible
		if !visible {
			// 音频继续播放时不能让视频队列堵住解封装，否则恢复后音频时钟
			// 已领先旧视频数分钟，导致持续丢帧。隐藏期间只丢弃视频输出。
			C.vt_core_flush_frames(core)
			next = 100 * time.Millisecond
			continue
		}
		if !idle && time.Since(lastFrame) > 10*time.Second {
			p.setError("直播画面长时间没有更新")
			continue
		}

		if !useAudio && base < 0 {
			var frame unsafe.Pointer
			var pts C.double
			if C.vt_core_take_newest(core, &frame, &pts) == 0 {
				continue
			}
			p.taken.Add(1)
			now := time.Now()
			presented := float64(pts)
			presentStart := time.Now()
			drawn := p.surface.present(frame)
			presentNS += time.Since(presentStart).Nanoseconds()
			C.vt_core_release_frame(frame)
			p.mu.Lock()
			p.basePts = presented
			p.started = now
			origin := p.originPts
			if !p.originSet {
				p.originPts = presented
				p.originSet = true
				origin = presented
			}
			if drawn {
				p.presentOK.Add(1)
				nowPresent := time.Now()
				if !lastPresent.IsZero() {
					gap := nowPresent.Sub(lastPresent)
					sumGap += gap
					gapCount++
					if gap > maxGap {
						maxGap = gap
					}
				}
				lastPresent = nowPresent
				p.lastPts = presented
				p.position = presented - origin
				p.lastFrameAt = time.Now()
				p.presented.Add(1)
			} else {
				p.presentFail.Add(1)
			}
			p.mu.Unlock()
			continue
		}

		target := base + time.Since(started).Seconds()
		if useAudio {
			target = clock
		}
		var oldest, newest C.double
		var count C.int
		C.vt_core_peek_pts(core, &oldest, &newest, &count)
		if count == 0 {
			if idle {
				return
			}
			continue
		}
		if useAudio && target > float64(newest)+1.0 {
			for count > 0 {
				var dropped unsafe.Pointer
				var droppedPts C.double
				if C.vt_core_take_oldest(core, &dropped, &droppedPts) == 0 {
					break
				}
				C.vt_core_release_frame(dropped)
				p.droppedLocal.Add(1)
				C.vt_core_peek_pts(core, nil, nil, &count)
			}
			continue
		}
		if !useAudio && target > float64(newest)+1.0 {
			// The clock outran the stream (pause, occlusion or a network
			// stall): jump to the live edge instead of dropping forever.
			p.mu.Lock()
			p.basePts = -1
			p.mu.Unlock()
			continue
		}
		for count > 1 && target-float64(oldest) > .30 {
			var dropped unsafe.Pointer
			var droppedPts C.double
			if C.vt_core_take_oldest(core, &dropped, &droppedPts) == 0 {
				break
			}
			C.vt_core_release_frame(dropped)
			p.droppedLocal.Add(1)
			C.vt_core_peek_pts(core, &oldest, nil, &count)
		}
		if count == 0 || float64(oldest) > target+.005 {
			next = time.Duration(max(.002, min(.05, float64(oldest)-target)) * float64(time.Second))
			continue
		}
		var frame unsafe.Pointer
		var pts C.double
		if C.vt_core_take_oldest(core, &frame, &pts) == 0 {
			continue
		}
		p.taken.Add(1)
		presented := float64(pts)
		presentStart := time.Now()
		drawn := p.surface.present(frame)
		presentNS += time.Since(presentStart).Nanoseconds()
		C.vt_core_release_frame(frame)
		if !drawn {
			p.presentFail.Add(1)
			continue
		}
		p.presentOK.Add(1)
		nowPresent := time.Now()
		if !lastPresent.IsZero() {
			gap := nowPresent.Sub(lastPresent)
			sumGap += gap
			gapCount++
			if gap > maxGap {
				maxGap = gap
			}
		}
		lastPresent = nowPresent
		p.mu.Lock()
		if !p.originSet {
			p.originPts = presented
			p.originSet = true
		}
		p.lastPts = presented
		p.position = presented - p.originPts
		p.lastFrameAt = time.Now()
		p.mu.Unlock()
		p.presented.Add(1)
	}
}

func (p *vtPlayer) setError(message string) {
	if message == "" {
		message = "播放失败"
	}
	p.mu.Lock()
	if p.errMsg == "" {
		p.errMsg = message
	}
	p.mu.Unlock()
}

func (p *vtPlayer) Stats() Stats {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	p.mu.Lock()
	core := p.core
	audio := p.audio
	audioFailed := p.audioFailed
	audioCodec := ""
	avSync := 0.0
	if audio != nil && !audioFailed {
		audioCodec = "aac"
		if clock := float64(C.vt_audio_clock(audio)); !math.IsNaN(clock) {
			avSync = clock - p.lastPts
		}
	}
	stats := Stats{
		Position:   p.position,
		Width:      p.width,
		Height:     p.height,
		FPS:        p.fps,
		Hardware:   "videotoolbox",
		VideoCodec: p.codec,
		AudioCodec: audioCodec,
		AVSync:     avSync,
		Dropped:    p.dropped,
		Visible:    p.surface.visible.Load(),
		Paused:     p.paused,
		Muted:      p.muted,
		Idle:       core == nil || (p.idle && C.vt_core_queue_count(core) == 0),
		Error:      p.errMsg,
	}
	if p.audioOnly {
		stats.Hardware = ""
		stats.VideoCodec = ""
		stats.FPS = 0
		stats.AVSync = 0
	}
	p.mu.Unlock()
	if core != nil {
		stats.Dropped = int(C.vt_core_dropped(core)) + int(p.droppedLocal.Load())
		stats.DecodedFrames = uint64(C.vt_core_decoded(core))
		if stats.Width == 0 {
			stats.Width = int(C.vt_core_width(core))
			stats.Height = int(C.vt_core_height(core))
		}
		var errBuf [512]C.char
		if C.vt_core_error(core, &errBuf[0], C.int(len(errBuf))) != 0 && stats.Error == "" {
			stats.Error = vtErrorMessage(errBuf[:])
		}
	}
	if stats.Error == "" {
		stats.Error = p.surface.RenderError()
	}
	return stats
}

func (p *vtPlayer) Command(args ...string) error {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	if p.closed {
		return errors.New("播放器已关闭")
	}
	if len(args) >= 2 && args[0] == "screenshot-to-file" {
		if strings.IndexByte(args[1], 0) >= 0 {
			return errors.New("保存路径无效")
		}
		path := C.CString(args[1])
		defer C.free(unsafe.Pointer(path))
		var message [512]C.char
		if C.vt_view_save_png(p.surface.view, path, &message[0], C.int(len(message))) != 0 {
			return errors.New(C.GoString(&message[0]))
		}
		return nil
	}
	if len(args) >= 2 && args[0] == "set" {
		switch args[1] {
		case "stream-record":
			if len(args) < 3 || strings.IndexByte(args[2], 0) >= 0 {
				return errors.New("保存路径无效")
			}
			p.mu.Lock()
			core := p.core
			p.mu.Unlock()
			var message [512]C.char
			var result C.int
			if args[2] == "" {
				result = C.vt_core_record_stop(core, &message[0], C.int(len(message)))
			} else {
				path := C.CString(args[2])
				defer C.free(unsafe.Pointer(path))
				result = C.vt_core_record_start(core, path, &message[0], C.int(len(message)))
			}
			if result != 0 {
				return errors.New(C.GoString(&message[0]))
			}
			return nil
		case "vid":
			only := len(args) > 2 && args[2] == "no"
			p.mu.Lock()
			changed := p.audioOnly != only
			p.audioOnly = only
			core := p.core
			target, headers := p.target, p.headers
			p.lastFrameAt = time.Now()
			p.mu.Unlock()
			if changed && !only && core != nil {
				return p.load(target, headers)
			}
			if core != nil {
				C.vt_core_flush_frames(core)
			}
			return nil
		case "pause":
			paused := len(args) > 2 && args[2] == "yes"
			p.mu.Lock()
			resumeLive := !paused && p.paused && (strings.HasPrefix(p.target, "https://") || strings.HasPrefix(p.target, "http://"))
			target, headers := p.target, p.headers
			p.mu.Unlock()
			// 直播暂停时服务器可能断开闲置的 HTTP 连接。续播重新打开流，
			// 保留旧画面直到新会话出帧，不在已经失效的读取状态上继续。
			if resumeLive {
				return p.load(target, headers)
			}
			p.mu.Lock()
			changed := p.paused != paused
			p.paused = paused
			core, audio := p.core, p.audio
			if changed {
				if paused {
					if audio != nil {
						C.vt_audio_set_rate(audio, 0)
					}
				} else {
					p.lastFrameAt = time.Now()
					p.basePts = -1
					p.audioWait = time.Time{}
					if audio != nil && !p.audioFailed {
						C.vt_audio_reset(audio)
						p.audioStarted = false
					}
				}
			}
			p.mu.Unlock()
			if changed && !paused && core != nil {
				C.vt_core_flush_frames(core)
			}
			return nil
		case "keepaspect":
			p.surface.keepAspect.Store(len(args) <= 2 || args[2] != "no")
			p.surface.applyFit()
			return nil
		case "panscan":
			p.surface.fillVideo.Store(len(args) > 2 && args[2] == "1")
			p.surface.applyFit()
			return nil
		case "mute":
			value := len(args) > 2 && args[2] == "yes"
			p.mu.Lock()
			p.muted = value
			audio := p.audio
			p.mu.Unlock()
			if audio != nil {
				C.vt_audio_set_muted(audio, boolInt(value))
			}
			return nil
		case "volume":
			p.mu.Lock()
			volume := p.volume
			p.mu.Unlock()
			if len(args) > 2 {
				if parsed, err := strconv.ParseFloat(args[2], 64); err == nil {
					volume = parsed
				}
			}
			p.mu.Lock()
			p.volume = max(0, min(100, volume))
			audio := p.audio
			p.mu.Unlock()
			if audio != nil {
				C.vt_audio_set_volume(audio, C.double(volume/100))
			}
			return nil
		}
		return fmt.Errorf("原生硬解不支持属性 %s", args[1])
	}
	if len(args) > 0 && args[0] == "stop" {
		p.stopSession()
		p.mu.Lock()
		p.idle = true
		p.target, p.headers = "", nil
		p.position = 0
		p.mu.Unlock()
		return nil
	}
	return fmt.Errorf("原生硬解不支持命令 %v", args)
}

func (p *vtPlayer) Close() {
	p.sessionMu.Lock()
	defer p.sessionMu.Unlock()
	p.closed = true
	p.stopSession()
	p.target, p.headers = "", nil
}

func (p *vtPlayer) stopSession() {
	p.mu.Lock()
	core := p.core
	audio := p.audio
	cancel := p.cancel
	p.core = nil
	p.audio = nil
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if core != nil {
		C.vt_core_stop(core)
	}
	p.wg.Wait()
	if audio != nil {
		C.vt_audio_close(audio)
	}
	if core != nil {
		C.vt_core_close(core)
	}
}

func vtHeader(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}

func vtHeaderBlock(headers map[string]string) string {
	keys := make([]string, 0, len(headers))
	for key := range headers {
		if strings.EqualFold(key, "User-Agent") || strings.EqualFold(key, "Referer") {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteString(": ")
		builder.WriteString(headers[key])
		builder.WriteString("\r\n")
	}
	return builder.String()
}

func vtCAFile() string {
	executable, err := os.Executable()
	if err != nil {
		return ""
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(executable), "../Resources/cacert.pem"),
		filepath.Join(filepath.Dir(executable), "cacert.pem"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func vtErrorMessage(buffer []C.char) string {
	message := C.GoString(&buffer[0])
	if message == "" {
		return "播放失败"
	}
	return message
}

func boolInt(value bool) C.int {
	if value {
		return 1
	}
	return 0
}
