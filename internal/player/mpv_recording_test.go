//go:build darwin || windows

package player

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMPVRecordingStartsWithDecodableKeyframe(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("需要 FFmpeg 生成和解码样本")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("需要 ffprobe 检查片段")
	}
	p, err := New(nil)
	if err != nil {
		if strings.Contains(err.Error(), "找不到 libmpv") || strings.Contains(err.Error(), "无法加载 Windows libmpv") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer p.Close()

	dir := t.TempDir()
	input, output := filepath.Join(dir, "input.flv"), filepath.Join(dir, "clip.mkv")
	data, err := exec.Command(ffmpeg, "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x90:rate=30",
		"-f", "lavfi", "-i", "sine=sample_rate=48000",
		"-t", "8", "-c:v", "libx264", "-g", "60", "-sc_threshold", "0", "-c:a", "aac", input).CombinedOutput()
	if err != nil {
		t.Fatalf("生成样本：%v %s", err, data)
	}
	if err := p.Load(input, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for p.Stats().Position < 1.25 {
		if time.Now().After(deadline) {
			t.Fatal("播放未推进到关键帧之间")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := p.StartRecording(output); err != nil {
		t.Fatal(err)
	}
	if err := p.StopRecording(output); err != nil {
		t.Fatal(err)
	}

	data, err = exec.Command(ffprobe, "-v", "error", "-select_streams", "v:0", "-read_intervals", "%+#1",
		"-show_entries", "packet=flags", "-of", "csv=p=0", output).CombinedOutput()
	if err != nil || !strings.HasPrefix(string(data), "K") {
		t.Fatalf("视频没有从关键帧开始：%v %s", err, data)
	}
	data, err = exec.Command(ffmpeg, "-v", "error", "-xerror", "-i", output, "-map", "0:v:0", "-map", "0:a:0",
		"-f", "null", "-").CombinedOutput()
	if err != nil || len(data) != 0 {
		t.Fatalf("音视频片段无法完整解码：%v %s", err, data)
	}
}
