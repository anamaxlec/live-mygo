package platforms

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestFLiveResolvesPublicPlaylistAndRelativeQualityWithoutAccount(t *testing.T) {
	c := NewClient(testCookies{})
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Cookie") != "" || req.Header.Get("Token") != "" {
			t.Fatal("赛事请求携带了其他来源的登录")
		}
		var body string
		switch req.URL.Host {
		case "cdn.flive.dpdns.org":
			body = `[{"id":1,"title":"F1 TV","uri":"https://embed.example/player","status":"online"},{"id":2,"title":"F1 TV HD","uri":"https://video.example/other.m3u8","status":"online"}]`
		case "embed.example":
			body = `<script>player({source:'https://video.example/live/master.m3u8?key=public'});</script>`
		case "video.example":
			body = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=6000000,RESOLUTION=1920x1080\n1080/index.m3u8?key=hd\n#EXT-X-STREAM-INF:BANDWIDTH=2500000,RESOLUTION=1280x720\n720/index.m3u8?key=sd\n"
		default:
			t.Fatal("意外请求", req.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	s, err := c.Resolve(context.Background(), FLive, "1", 2)
	if err != nil || s.URL != "https://video.example/live/720/index.m3u8?key=sd" || len(s.Qualities) != 3 || s.Qualities[1].Name != "1080p" || len(s.Sources) != 2 {
		t.Fatal(s, err)
	}
	if s.Headers["Referer"] != "https://embed.example/player" {
		t.Fatal("播放请求未保留播放器来源", s.Headers)
	}
	if _, id, err := ParseRoom(Bilibili, "https://flive.dpdns.org/player?channelId=1"); err != nil || id != "1" {
		t.Fatal(id, err)
	}
}

func TestFLiveRejectsMissingSignalAndPrivateAddress(t *testing.T) {
	for _, target := range []string{"file:///tmp/video.m3u8", "https://localhost/live.m3u8", "https://127.0.0.1/live.m3u8", "https://192.168.1.1/live.m3u8"} {
		if _, err := publicStreamURL(target); err == nil {
			t.Fatal("接受了私有或非媒体协议地址", target)
		}
	}
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("<html>no signal</html>"))}, nil
	})
	if _, _, _, err := c.flivePlaylist(context.Background(), "https://media.example/", flivePage, 0); err == nil {
		t.Fatal("将无媒体的网页当成视频")
	}
}

func TestFLiveMissingSignalKeepsRoomAndRoutes(t *testing.T) {
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		status, body := 404, ""
		if req.URL.Host == "cdn.flive.dpdns.org" {
			status = 200
			body = `[{"id":1,"title":"F1 TV","uri":"https://video.example/a.m3u8","status":"online"},{"id":2,"title":"F1 TV HD","uri":"https://video.example/b.m3u8","status":"online"}]`
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	s, err := c.Resolve(context.Background(), FLive, "1", -1)
	if err == nil || s == nil || s.Room.Title != "F1 TV" || len(s.Sources) != 2 || s.URL != "" {
		t.Fatal("信号失败后丢失频道身份或换线选项", s, err)
	}
}
