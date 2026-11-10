//go:build darwin || windows

package app

import (
	"testing"

	"github.com/egoist/mygo/transfer"
	"live-mygo/internal/platforms"
)

func TestDroppedLiveRoom(t *testing.T) {
	for _, tt := range []struct {
		name   string
		data   transfer.Data
		source platforms.Platform
		id     string
	}{
		{"浏览器 URL 优先于文字", transfer.New(transfer.NewItem(
			transfer.Bytes(transfer.URIList, []byte("# URL\r\nhttps://live.bilibili.com/123?from=web\r\n")),
			transfer.Bytes(transfer.Text, []byte("https://www.douyu.com/456")),
		)), platforms.Bilibili, "123"},
		{"聊天分享文字", transfer.TextData("来看直播：https://www.douyu.com/456。"), platforms.Douyu, "456"},
		{"非直播网址", transfer.TextData("https://example.com/?next=https://live.bilibili.com/123"), "", ""},
		{"伪造的来源域名", transfer.TextData("https://live.bilibili.com.example.com/123"), "", ""},
		{"文件链接", transfer.New(transfer.NewItem(transfer.Bytes(transfer.URIList, []byte("file:///tmp/test.mp4")))), "", ""},
		{"纯房间号", transfer.TextData("123"), "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			room, ok := droppedLiveRoom(tt.data)
			if ok != (tt.id != "") || room.Platform != tt.source || room.ID != tt.id {
				t.Fatalf("droppedLiveRoom = %+v, %t；预期 %s/%s", room, ok, tt.source, tt.id)
			}
		})
	}
}
