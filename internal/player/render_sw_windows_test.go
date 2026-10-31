//go:build windows && (amd64 || arm64)

package player

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSurfaceBufferPoolSurvivesResize(t *testing.T) {
	s, err := NewSurface(1)
	if err != nil {
		t.Fatal(err)
	}
	var small [][]byte
	for i := 0; i < maxFrameBuffers; i++ {
		buf, _, ok := s.acquire(100, 100)
		if !ok {
			t.Fatalf("第 %d 个缓冲分配失败", i)
		}
		small = append(small, buf)
	}
	for _, buf := range small {
		s.ReleaseFrame(buf)
	}
	buf, stride, ok := s.acquire(640, 360)
	if !ok {
		t.Fatal("尺寸变大后缓冲池被旧尺寸缓冲占满")
	}
	if stride != 640*4 || len(buf) != 640*360*4 {
		t.Fatalf("缓冲尺寸错误：stride=%d len=%d", stride, len(buf))
	}
	s.ReleaseFrame(buf)
}

func TestSWRenderFrame(t *testing.T) {
	video, _ := filepath.Abs("../../build/windows-amd64/test-pattern.mp4")
	if _, err := os.Stat(video); err != nil {
		t.Skip("没有测试视频")
	}
	s, err := NewSurface(1)
	if err != nil {
		t.Fatal(err)
	}
	s.width.Store(640)
	s.height.Store(360)
	s.visible.Store(true)
	s.dirty.Store(true)
	p, err := New(s)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if err := p.Load(video, nil); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if pix, w, h, stride, ok := s.TakeFrame(); ok {
			t.Logf("frame %dx%d stride=%d first=%v", w, h, stride, pix[:8])
			for i := 3; i < len(pix); i += 4 {
				if pix[i] != 255 {
					t.Fatalf("像素 %d 的 alpha=%d，期望 255", i/4, pix[i])
				}
			}
			s.ReleaseFrame(pix)
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no frame")
}
