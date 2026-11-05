//go:build darwin && cgo

package player

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestNativeRecordingCopiesAudioVideoAndFinalizesOnStop(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg 生成样本")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("需要 ffprobe 检查片段")
	}
	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.mp4"), filepath.Join(dir, "clip.mkv")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-f", "lavfi", "-i", "sine=sample_rate=48000", "-t", "4", "-c:v", "libx264", "-g", "15", "-c:a", "aac", input).CombinedOutput()
	if err != nil {
		t.Fatalf("生成样本：%v %s", err, data)
	}
	surface := &VTSurface{}
	// 按前台的视频时钟消费样本。隐藏视图会主动丢弃视频输出，
	// 四秒本地文件可能在录制断言前读完，EOF 不能用来判断录制是否停止播放。
	surface.visible.Store(true)
	p := &vtPlayer{surface: surface, volume: 0}
	defer p.Close()
	if err := p.Load(input, nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Command("set", "stream-record", output); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1200 * time.Millisecond)
	if err := p.Command("set", "stream-record", ""); err != nil {
		t.Fatal(err)
	}
	if err := p.Command("set", "stream-record", ""); err != nil {
		t.Fatal("重复停止", err)
	}
	data, err = exec.Command(ffprobe, "-v", "error", "-show_entries", "stream=codec_name:format=duration", "-of", "json", output).CombinedOutput()
	if err != nil {
		t.Fatalf("片段无法读取：%v %s", err, data)
	}
	var report struct {
		Streams []struct {
			Codec string `json:"codec_name"`
		}
		Format struct{ Duration string }
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Streams) != 2 || report.Streams[0].Codec != "h264" || report.Streams[1].Codec != "aac" || report.Format.Duration == "" || report.Format.Duration == "0.000000" {
		t.Fatal(string(data))
	}
	if stats := p.Stats(); stats.Error != "" || stats.Idle {
		t.Fatal("录制影响了播放", stats)
	}
	if err := p.Command("set", "stream-record", filepath.Join(dir, "missing", "fail.mkv")); err == nil {
		t.Fatal("未报告无效保存位置")
	}
	if stats := p.Stats(); stats.Error != "" || stats.Idle {
		t.Fatal("录制失败影响了播放", stats)
	}
}

func TestNativePlayerRejectsTruncatedRequestsAndClosedCommands(t *testing.T) {
	p := &vtPlayer{surface: &VTSurface{}}
	if err := p.Load("https://example.com/a\x00b", nil); err == nil {
		t.Fatal("NUL 地址未拒绝")
	}
	if err := p.Load("https://example.com/a", map[string]string{"Cookie": "x\r\nOther: value"}); err == nil {
		t.Fatal("换行请求头未拒绝")
	}
	p.Close()
	if err := p.Command("set", "pause", "yes"); err == nil {
		t.Fatal("已关闭播放器仍接受命令")
	}
	if err := p.Load("file.mp4", nil); err == nil {
		t.Fatal("已关闭播放器仍接受加载")
	}
}

func TestNativeAudioOnlyStopsVideoDecodingAndCanRestoreIt(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg 生成样本")
	}
	path := filepath.Join(t.TempDir(), "audio-only.mp4")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "3", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成样本：%v %s", err, data)
	}
	p := &vtPlayer{surface: &VTSurface{}, volume: 0}
	defer p.Close()
	if err := p.Command("set", "vid", "no"); err != nil {
		t.Fatal(err)
	}
	if err := p.Load(path, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.Stats().Position < .1 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	stats := p.Stats()
	if stats.Error != "" || stats.AudioCodec != "aac" || stats.Position < .1 || stats.DecodedFrames != 0 || stats.Hardware != "" {
		t.Fatal("只听声音仍在解码视频或音频没有推进", stats)
	}
	if err := p.Command("set", "vid", "auto"); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(2 * time.Second)
	for p.Stats().DecodedFrames == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if stats = p.Stats(); stats.Error != "" || stats.DecodedFrames == 0 {
		t.Fatal("恢复画面后没有重新解码视频", stats)
	}
}

func TestNativeDecoderCanStopWithPendingFrames(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg 生成解码样本")
	}
	path := filepath.Join(t.TempDir(), "sample.mp4")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成样本：%v %s", err, data)
	}
	p := &vtPlayer{surface: &VTSurface{}, volume: 60}
	defer p.Close()
	for i := 0; i < 3; i++ {
		if err := p.Load(path, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		if err := p.Command("set", "pause", "yes"); err != nil {
			t.Fatal(err)
		}
		if err := p.Command("set", "pause", "no"); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() { _ = p.Command("stop"); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("待显示帧没有被取消，停止阻塞")
		}
		if !p.Stats().Idle {
			t.Fatal("停止后仍报告播放")
		}
	}
}

func TestNativeDemuxRejectsUntrustedTLS(t *testing.T) {
	var reached atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
		_, _ = w.Write([]byte("not a media file"))
	}))
	defer server.Close()
	p := &vtPlayer{surface: &VTSurface{}}
	defer p.Close()
	if err := p.Load(server.URL+"/live.flv", nil); err == nil {
		t.Fatal("不应接受不受信任证书")
	}
	if reached.Load() {
		t.Fatal("不受信任证书已被绕过并请求媒体")
	}
}

func TestNativeLiveResumeReopensHTTPStream(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg")
	}
	path := filepath.Join(t.TempDir(), "sample.mp4")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-t", "2", "-c:v", "libx264", "-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成样本：%v %s", err, data)
	}
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); http.ServeFile(w, r, path) }))
	defer server.Close()
	p := &vtPlayer{surface: &VTSurface{}, volume: 60}
	defer p.Close()
	if err := p.Load(server.URL+"/sample.mp4", nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Command("set", "pause", "yes"); err != nil {
		t.Fatal(err)
	}
	before := requests.Load()
	if err := p.Command("set", "pause", "no"); err != nil {
		t.Fatal(err)
	}
	if requests.Load() <= before {
		t.Fatal("直播续播仍复用可能已失效的 HTTP 读取")
	}
	if p.Stats().Paused {
		t.Fatal("续播后仍暂停")
	}
}

func TestNativeStartupConsumesVideoBeforeDelayedAudio(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg")
	}
	path := filepath.Join(t.TempDir(), "delayed-audio.mp4")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30", "-itsoffset", "1", "-f", "lavfi", "-i", "sine=frequency=400:sample_rate=48000", "-t", "2", "-c:v", "libx264", "-c:a", "aac", "-pix_fmt", "yuv420p", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成音视频样本：%v %s", err, data)
	}
	surface := &VTSurface{}
	surface.visible.Store(true)
	p := &vtPlayer{surface: surface, volume: 0}
	defer p.Close()
	if err := p.Load(path, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		started, failed := p.audioStarted, p.audioFailed
		p.mu.Unlock()
		if failed {
			t.Fatal("等待首音频时错误禁用了音轨")
		}
		if started {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("先到达的视频包阻塞了首个音频包")
}

func TestNativeAudioClockStartsWithBoundedVideoQueue(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg")
	}
	path := filepath.Join(t.TempDir(), "av-clock.mp4")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=60", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "8", "-c:v", "libx264", "-bf", "0", "-c:a", "aac", "-ac", "2", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成音视频样本：%v %s", err, data)
	}
	surface := &VTSurface{}
	surface.visible.Store(true)
	p := &vtPlayer{surface: surface, volume: 0}
	defer p.Close()
	if err := p.Load(path, nil); err != nil {
		t.Fatal(err)
	}
	// 检查实际时钟推进，不能以已提交首包或存在 AAC 音轨代替开始播放。
	deadline := time.Now().Add(1500 * time.Millisecond)
	var first float64
	for time.Now().Before(deadline) {
		stats := p.Stats()
		if stats.Error != "" {
			t.Fatal(stats.Error)
		}
		if stats.AudioClock >= .1 {
			first = stats.AudioClock
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if first == 0 {
		t.Fatal("视频队列阻塞了解封装，系统音频时钟未启动", p.Stats())
	}
	started := time.Now()
	time.Sleep(600 * time.Millisecond)
	stats := p.Stats()
	if advance := stats.AudioClock - first; advance < .45 || advance-time.Since(started).Seconds() > .15 {
		t.Fatalf("音频时钟未连续推进：%.3f 秒", advance)
	}
	// 硬解与音频设备的首次初始化可错开，等待实际预填完成，而非假定第 600 ms 已填满。
	deadline = time.Now().Add(time.Second)
	for stats.AudioBuffered < .4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
		stats = p.Stats()
	}
	if stats.AudioBuffered < .4 || stats.AudioRate < .99 {
		t.Fatal("音频仍被小型视频队列限制，未形成独立缓冲", stats)
	}
	for i := 0; i < 3; i++ {
		before, at := p.Stats().AudioClock, time.Now()
		surface.visible.Store(false)
		time.Sleep(120 * time.Millisecond)
		if stats = p.Stats(); stats.VideoAhead > .2 {
			t.Fatal("后台清空了未来帧，切回前台仍需等待视频时钟追赶", stats)
		}
		surface.visible.Store(true)
		time.Sleep(120 * time.Millisecond)
		stats = p.Stats()
		if delta := stats.AudioClock - before - time.Since(at).Seconds(); delta < -.1 || delta > .1 {
			t.Fatal("窗口恢复可见时音频时钟中断或跳跃", stats)
		}
	}
	if err := p.Command("set", "pause", "yes"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	paused := p.Stats().AudioClock
	time.Sleep(100 * time.Millisecond)
	if delta := p.Stats().AudioClock - paused; delta < -.02 || delta > .02 {
		t.Fatal("暂停时音频时钟仍在推进", delta)
	}
	if err := p.Command("set", "pause", "no"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if delta := p.Stats().AudioClock - paused; delta < .1 || delta > .35 {
		t.Fatal("本地续播清空了音频，未从暂停位置继续", delta)
	}
	t.Logf("音频状态：%+v", stats)
}

func TestNativeFLVAudioKeepsAnExactSampleTimeline(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg")
	}
	path := filepath.Join(t.TempDir(), "quantized-audio.flv")
	data, err := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=60", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "5", "-c:v", "libx264", "-bf", "0", "-c:a", "aac", "-ac", "2", "-f", "flv", path).CombinedOutput()
	if err != nil {
		t.Fatalf("生成毫秒量化的 FLV 样本：%v %s", err, data)
	}
	surface := &VTSurface{}
	surface.visible.Store(true)
	p := &vtPlayer{surface: surface, volume: 0}
	defer p.Close()
	if err := p.Load(path, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for p.Stats().AudioClock < .1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if p.Stats().AudioClock < .1 {
		t.Fatal("FLV 音频时钟未启动")
	}
	for i := 0; i < 16; i++ {
		stats := p.Stats()
		// 此样本首个 AAC 时间戳为 0；每个缓冲尾部必须落在 1024 样本的网格上。
		packets := (stats.AudioClock + stats.AudioBuffered) * 48000 / 1024
		if stats.Error != "" || stats.AudioBuffered <= 0 || math.Abs(packets-math.Round(packets)) > .0001 {
			t.Fatalf("AAC 缓冲仍使用 21 / 22 ms 的量化时间戳，存在重叠或间隙：%+v", stats)
		}
		time.Sleep(37 * time.Millisecond)
	}
}
