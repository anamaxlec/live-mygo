//go:build darwin || windows

package app

import (
	"bytes"
	"context"
	"errors"
	"github.com/egoist/mygo/ui"
	"image"
	"image/png"
	"live-mygo/internal/library"
	"live-mygo/internal/platforms"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestRoomChangeCancelsAndRejectsOldChat(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &app{chatCancel: cancel, chatGeneration: 1, chatRoomKey: "bilibili:6", chatStatus: "已连接", chatMessages: []platforms.ChatMessage{{Text: "旧房间"}}}
	a.resetChat("等待直播间连接")
	if ctx.Err() == nil || len(a.chatMessages) != 0 || a.chatRoomKey != "" || a.chatCancel != nil {
		t.Fatal("切房未隔离旧会话")
	}
	a.acceptChatMessages(1, platforms.Bilibili, []platforms.ChatMessage{{Text: "迟到的旧消息"}}, 1)
	if len(a.chatMessages) != 0 || a.chatDropped != 0 {
		t.Fatal("旧回调污染了新房间")
	}
	a.acceptChatMessages(2, platforms.Douyu, []platforms.ChatMessage{{Text: "新消息"}}, 2)
	if len(a.chatMessages) != 1 || a.chatDropped != 2 {
		t.Fatal("新会话无法接收")
	}
}
func TestChatRetentionProtectsSuperChatFromOrdinaryFlood(t *testing.T) {
	data := []platforms.ChatMessage{{ID: 1, Kind: "superchat", Text: "支持"}}
	for i := 2; i < 1002; i++ {
		data = retainChat(data, []platforms.ChatMessage{{ID: uint64(i), Text: "普通消息"}})
	}
	if len(data) != 501 || data[0].Kind != "superchat" || data[1].ID != 502 {
		t.Fatal("醒目留言被普通消息挤掉或锚点顺序错误")
	}
}
func TestDuplicateFilterMergesAcrossUsersButKeepsSuperChat(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	state.Preferences.FilterDuplicates = true
	a := &app{library: state}
	a.acceptChatMessages(0, platforms.Bilibili, []platforms.ChatMessage{{User: "A", Text: "hello  直播"}, {User: "B", Text: "HELLO 直播"}, {User: "B", Text: "hello 直播", Kind: "superchat"}}, 0)
	if len(a.chatMessages) != 2 || a.chatMessages[1].Kind != "superchat" {
		t.Fatal("重复过滤或醒目留言保留错误")
	}
}
func TestChatReconnectIsSerialBoundedAndCancelable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts := 0
	reconnectChat(ctx, func(ready func()) error { attempts++; return errors.New("断线") }, func(string) {}, func(int) time.Duration { return time.Microsecond })
	if attempts != 9 {
		t.Fatalf("连续失败没有停止：%d", attempts)
	}
	attempts = 0
	reconnectChat(ctx, func(ready func()) error { attempts++; cancel(); return errors.New("断线") }, func(string) { t.Fatal("取消后还在重连") }, chatRetryDelay)
	if attempts != 1 {
		t.Fatal("取消未停止循环")
	}
}
func TestFailedCoverCanRecoverWithoutPermanentLoading(t *testing.T) {
	var imageData bytes.Buffer
	png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 20, 10)))
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(503)
			return
		}
		w.Write(imageData.Bytes())
	}))
	defer server.Close()
	a := &app{coverLoading: map[string]bool{server.URL: true}, covers: map[string]*ui.Bitmap{}}
	img, err := downloadCover(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("错误状态被当成图片")
	}
	a.finishCover(server.URL, img)
	if a.coverLoading[server.URL] || !time.Now().Before(a.coverRetry[server.URL]) {
		t.Fatal("失败仍占用加载标记或没有冷却")
	}
	img, err = downloadCover(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.finishCover(server.URL, img)
	if a.covers[server.URL] == nil || !a.coverRetry[server.URL].IsZero() {
		t.Fatal("图片无法恢复")
	}
}
func TestFavoritesSortsLiveFirstWithoutChangingSavedOrder(t *testing.T) {
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	state.Favorites = []platforms.Room{{Platform: platforms.Bilibili, ID: "1"}, {Platform: platforms.Douyu, ID: "2"}}
	a := &app{library: state, libraryRooms: map[string]platforms.Room{"douyu:2": {Platform: platforms.Douyu, ID: "2", Live: true}}}
	if a.favoriteRooms()[0].ID != "2" || state.Favorites[0].ID != "1" {
		t.Fatal("直播排序错误或改变了本机收藏顺序")
	}
}

func TestCaptionSpriteBackgroundWorkersFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	state, _ := library.Load(filepath.Join(t.TempDir(), "library.json"))
	a := &app{ctx: ctx}
	var sprites []*captionSprite
	for range 8 {
		sprites = append(sprites, a.captionSprite("测试 Aa", 100, 40, ui.Hex("#ffffff"), state.Preferences))
	}
	end := time.Now().Add(3 * time.Second)
	for _, s := range sprites {
		for s.bitmap.Load() == nil && time.Now().Before(end) {
			time.Sleep(time.Millisecond)
		}
		if s.bitmap.Load() == nil {
			t.Fatal("后台字图未完成")
		}
	}
}
