package platforms

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHuyaSearchIgnoresUnrelatedArraySections(t *testing.T) {
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"response":{"1026":[],"3":{"docs":[{"room_id":660000,"game_introduction":"比赛","game_nick":"赛事主播","game_screenshot":"https://example.com/cover.jpg"}],"numFound":31}}}`))}, nil
	})
	result, err := c.Search(context.Background(), Huya, "比赛", 1)
	if err != nil || len(result.Rooms) != 1 || result.Rooms[0].ID != "660000" || !result.HasMore {
		t.Fatalf("混合结构的搜索结果解析失败：%+v %v", result, err)
	}
}

func TestHuyaChatMessageAndTruncatedPackets(t *testing.T) {
	var sender, format, body, push, packet tars
	sender.number(0, 1234)
	sender.str(2, "观众")
	format.number(0, 0xabcdef)
	body.object(0, sender)
	body.str(3, "测试弹幕")
	body.object(6, format)
	push.number(1, 1400)
	push.data(2, body)
	packet.number(0, 7)
	packet.data(1, push)
	got, ok := decodeHuyaMessage(packet)
	if !ok || got.User != "观众" || got.Text != "测试弹幕" || got.UserID != "1234" || got.Color != 0xabcdef {
		t.Fatal("弹幕解码错误", got)
	}
	for i := 0; i < len(packet); i++ {
		if _, ok := decodeHuyaMessage(packet[:i]); ok {
			t.Fatal("截断数据被当成有效消息", i)
		}
	}
}
func TestHuyaLinksAndAntiCode(t *testing.T) {
	p, id, err := ParseRoom(Bilibili, "https://www.huya.com/660000")
	if err != nil || p != Huya || id != "660000" {
		t.Fatal(p, id, err)
	}
	q, err := huyaAnti("stream", 1234, "fm=cHJlZml4XyQwXyQxXyQyXyQz&wsTime=ffffffff&ctype=huya_pc_exe&t=100&fs=test", time.Unix(1700000000, 0))
	if err != nil || len(q.Get("wsSecret")) != 32 || q.Get("u") != "315904" {
		t.Fatal("签名字段错误", q, err)
	}
}
