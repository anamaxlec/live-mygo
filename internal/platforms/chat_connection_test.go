package platforms

import (
	"context"
	"errors"
	"fmt"
	"github.com/coder/websocket"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

type chatTestTransport func(*http.Request) (*http.Response, error)

func (f chatTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDouyuFailoverUsesReferencesAndConfirmsSubscription(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"www.douyu.com"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		for _, expected := range []string{"type@=loginreq/roomid@=24422/", "type@=joingroup/rid@=24422/gid@=-9999/"} {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if string(data[12:len(data)-1]) != expected {
				t.Errorf("订阅包不一致：%q", data)
			}
		}
		conn.Write(ctx, websocket.MessageBinary, douyuPacket("type@=loginres/"))
		conn.Write(ctx, websocket.MessageBinary, douyuPacket("type@=chatmsg/rid@=24422/nn@=观众/txt@=hello/dms@=1/"))
		<-ctx.Done()
	}))
	defer server.Close()
	local, _ := url.Parse(server.URL)
	var ports []string
	c := NewClient(nil)
	c.HTTP = &http.Client{Transport: chatTestTransport(func(r *http.Request) (*http.Response, error) {
		ports = append(ports, r.URL.Port())
		if len(ports) < 3 {
			return nil, errors.New("TLS handshake failure")
		}
		r.URL.Scheme = local.Scheme
		r.URL.Host = local.Host
		r.Host = local.Host
		return http.DefaultTransport.RoundTrip(r)
	})}
	ready, received := false, false
	err := c.ListenChat(ctx, Douyu, "24422", func(m ChatMessage) { received = m.Text == "hello"; cancel() }, func(s string) {
		if s == "已连接" {
			ready = true
		}
	})
	if !ready || !received || !reflect.DeepEqual(ports, []string{"8501", "8502", "8503"}) {
		t.Fatalf("没有正确换端点或订阅：%v %v %v %v", ports, ready, received, err)
	}
}

func TestDouyuSuperChatAndRoomIsolation(t *testing.T) {
	escape := func(s string) string { return strings.NewReplacer("@", "@A", "/", "@S").Replace(s) }
	nested := "nn@=观众/txt@=支持/uid@=42/"
	packet := douyuPacket("type@=comm_chatmsg/rid@=24422/cprice@=3000/chatmsg@=" + escape(nested) + "/")
	old := douyuPacket("type@=chatmsg/rid@=666/nn@=旧房间/txt@=错误消息/")
	var got []ChatMessage
	_, err := decodeDouyu(append(old, packet...), func(m ChatMessage) { got = append(got, m) }, func() {}, "24422")
	if err != nil || len(got) != 1 || got[0].Kind != "superchat" || got[0].Price != 30 || got[0].Text != "支持" {
		t.Fatalf("斗鱼醒目留言/房间隔离失败：%+v %v", got, err)
	}
}

func TestRoomInfoReadsMetadataWithoutStreamSigning(t *testing.T) {
	for _, p := range []Platform{Bilibili, Douyu} {
		t.Run(string(p), func(t *testing.T) {
			c := NewClient(nil)
			count := 0
			c.HTTP = &http.Client{Transport: chatTestTransport(func(r *http.Request) (*http.Response, error) {
				count++
				body := `{"code":0,"data":{"room_id":6,"live_status":1,"title":"test","keyframe":"https://example.com/cover.png"}}`
				if p == Douyu {
					body = `{"room":{"room_id":24422,"show_status":2,"room_name":"test","nickname":"anchor"}}`
				}
				if !strings.Contains(r.URL.Path, "get_info") && !strings.Contains(r.URL.Path, "betard") {
					t.Error("刷新关注不应取流")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
			})}
			r, err := c.RoomInfo(context.Background(), p, map[Platform]string{Bilibili: "6", Douyu: "24422"}[p])
			if err != nil || count != 1 || r.Title != "test" || r.Live != (p == Bilibili) {
				t.Fatal(fmt.Sprintf("元信息失败：%+v %v", r, err))
			}
		})
	}
}
