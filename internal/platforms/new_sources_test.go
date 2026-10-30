package platforms

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestNewSourceLinksAndQieSignature(t *testing.T) {
	for _, tc := range []struct {
		URL      string
		Platform Platform
		ID       string
	}{
		{"https://live.douyin.com/172066210996", Douyin, "172066210996"},
		{"https://www.qie.tv/10208562", Qie, "10208562"},
		{"https://live.qq.com/10208562", Qie, "10208562"},
	} {
		p, id, err := ParseRoom(Bilibili, tc.URL)
		if err != nil || p != tc.Platform || id != tc.ID {
			t.Fatalf("%s: %s %s %v", tc.URL, p, id, err)
		}
	}
	if _, _, err := ParseRoom(Douyin, "https://live.douyin.com.example.com/123"); err == nil {
		t.Fatal("accepted unrelated host")
	}
	if got := qieSignature("10005196", "30000000"); got != "1d654fbfdd32c6f4cf12562af23c2b70" {
		t.Fatal("官网签名样本不匹配", got)
	}
}

func TestFlightDataSupportsBothPagePrefixes(t *testing.T) {
	for _, prefix := range []string{"__pace_f", "__next_f"} {
		args, _ := json.Marshal([]any{1, `e:["$","$L13",null,{"state":{"roomStore":{"roomInfo":{"roomId":"123","room":{"title":"标题中含有 ] 和引号 \"","status":2}}}}}]`})
		page := []byte(`<script nonce="test">self.` + prefix + `.push(` + string(args) + `)</script>`)
		var state douyinPageState
		if err := flightValue(page, "state", &state); err != nil || state.RoomStore.Info.ID != "123" || !strings.Contains(state.RoomStore.Info.Room.Title, "引号") {
			t.Fatal(prefix, state, err)
		}
	}
}

func TestDouyinUsesPageQualityURLsWithoutSendingCookiesToCDN(t *testing.T) {
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		header := make(http.Header)
		body := ""
		if req.URL.Host == "ttwid.bytedance.com" {
			header.Set("Set-Cookie", "ttwid=anonymous-test; Domain=.douyin.com; Path=/; Secure")
		} else {
			if !strings.Contains(req.Header.Get("Cookie"), "ttwid=anonymous-test") {
				t.Fatal("缺少独立匿名会话")
			}
			room := `e:["$",null,null,{"state":{"roomStore":{"roomInfo":{"roomId":"999","room":{"status":2,"title":"测试","stream_url":{"flv_pull_url":{"FULL_HD1":"https://cdn.example.com/original.flv","SD1":"https://cdn.example.com/hd.flv"},"hls_pull_url_map":{"FULL_HD1":"https://cdn.example.com/original.m3u8"}}}}}}}]`
			args, _ := json.Marshal([]any{1, room})
			body = `<script>self.__pace_f.push(` + string(args) + `)</script>`
		}
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	s, err := c.Resolve(context.Background(), Douyin, "123", 2)
	if err != nil || s.QualityID != 2 || s.URL != "https://cdn.example.com/hd.flv" {
		t.Fatal(s, err)
	}
	if s.Headers["Cookie"] != "" {
		t.Fatal("匿名会话泄露至 CDN")
	}
}

func TestQieFrameAndProtobufRejectTruncation(t *testing.T) {
	packet := qiePacket(2000, []byte(`{"from":{"u":1,"n":"观众"},"msg":{"content":"消息"}}`))
	for i := 0; i < len(packet); i++ {
		called := false
		qieFrames(packet[:i], 0, func(uint32, []byte) { called = true })
		if called {
			t.Fatal("accepted truncated Qie frame", i)
		}
	}
	called := false
	qieFrames(packet, 0, func(op uint32, _ []byte) { called = op == 2000 })
	if !called {
		t.Fatal("valid Qie frame lost")
	}
	proto := protoBytes(nil, 8, []byte("payload"))
	proto = protoNumber(proto, 2, 42)
	fields, err := readProto(proto)
	if err != nil || fields.first(2).Number != 42 || string(fields.first(8).Bytes) != "payload" {
		t.Fatal(fields, err)
	}
	if _, err = readProto([]byte{0x42, 0xff, 0xff}); err == nil {
		t.Fatal("accepted invalid protobuf size")
	}
}

func TestQieResolveUsesServerSourceAndFailsWithoutAddress(t *testing.T) {
	for _, withURL := range []bool{true, false} {
		c := NewClient(nil)
		c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
			body := `<script id="__NEXT_DATA__">{"props":{"initialState":{"roomInfo":{"roomInfo":{"room_info":{"room_id":"1","room_name":"房间","show_status":"1"}}}}}}</script>`
			if strings.Contains(req.URL.Path, "swf_api") {
				q := req.URL.Query()
				if q.Get("sign") != qieSignature("1", q.Get("_t")) {
					t.Fatal("签名参数不匹配")
				}
				data := map[string]any{"rtmp_url": "", "rtmp_live": "", "rtmp_cdn": "bak2", "cdns": []string{"ws", "bak2"}}
				if withURL {
					data["rtmp_url"] = "https://cdn.example.com/live"
					data["rtmp_live"] = "video.flv?token=test"
				}
				b, _ := json.Marshal(map[string]any{"error": 0, "data": data})
				body = string(b)
			}
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		s, err := c.Resolve(context.Background(), Qie, "1", -1, "bak2")
		if withURL {
			if err != nil || s.SourceID != "bak2" || s.URL != "https://cdn.example.com/live/video.flv?token=test" {
				t.Fatal(s, err)
			}
		} else if err == nil || s != nil {
			t.Fatal("空地址被当成可播放", s, err)
		}
	}
}

func TestDouyinChatResponseKeepsUserIdentity(t *testing.T) {
	user := protoNumber(nil, 1, 123)
	user = protoBytes(user, 3, []byte("观众"))
	chat := protoBytes(nil, 2, user)
	chat = protoBytes(chat, 3, []byte("中文弹幕"))
	envelope := protoBytes(nil, 1, []byte("WebcastChatMessage"))
	envelope = protoBytes(envelope, 2, chat)
	response, err := douyinResponse(protoBytes(nil, 1, envelope))
	if err != nil {
		t.Fatal(err)
	}
	var got []ChatMessage
	visitDouyinMessages(response, func(m ChatMessage) { got = append(got, m) })
	if len(got) != 1 || got[0].UserID != "123" || got[0].User != "观众" || got[0].Text != "中文弹幕" {
		t.Fatal(got)
	}
}

type qieTestCookies struct{}

func (qieTestCookies) Header(p Platform, target string) (string, error) {
	if p == Qie && strings.HasPrefix(target, "https://www.qie.tv/") {
		return "token=synthetic-session; tokenId=synthetic-id", nil
	}
	return "", nil
}

func TestQieLoginAndQualityUseOfficialResponse(t *testing.T) {
	c := NewClient(qieTestCookies{})
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		if !strings.Contains(req.Header.Get("Cookie"), "token=synthetic-session") {
			t.Fatal("官网请求没有使用已保存的登录")
		}
		body := `<script id="__NEXT_DATA__">{"props":{"initialState":{"roomInfo":{"roomInfo":{"room_info":{"room_id":"1","show_status":"1"}}}}}}</script>`
		if strings.Contains(req.URL.Path, "swf_api") {
			if req.Header.Get("Token") != "synthetic-session" {
				t.Fatal("取流请求缺少官网要求的 token 请求头")
			}
			body = `{"error":0,"data":{"rtmp_url":"https://cdn.example.com/live","rtmp_live":"original.flv?sig=base","rtmp_multi_bitrate":{"2":"hd-specific.flv?sig=hd","1":"high-specific.flv?sig=high"}}}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	s, err := c.Resolve(context.Background(), Qie, "1", 1)
	if err != nil || len(s.Qualities) != 3 || s.URL != "https://cdn.example.com/live/high-specific.flv?sig=high" {
		t.Fatal("未使用接口返回的画质地址", s, err)
	}
	if s.Headers["Cookie"] != "" || s.Headers["Token"] != "" {
		t.Fatal("账号凭据被附加到视频 CDN")
	}
}
