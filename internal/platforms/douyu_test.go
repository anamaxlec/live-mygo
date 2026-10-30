package platforms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDouyuQualityKeepsRequestedRate(t *testing.T) {
	c := NewClient(nil)
	var rates []string
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		switch {
		case strings.HasSuffix(req.URL.Path, "/betard/42"):
			body = `{"room":{"room_id":"42","room_name":"测试直播","nickname":"主播","show_status":1}}`
		case req.URL.Path == "/swf_api/homeH5Enc":
			body = `{"error":0,"data":{"room42":"function ub98484234(rid,did,tt){return 'v=1&did='+did+'&tt='+tt+'&sign=test'}"}}`
		case strings.HasSuffix(req.URL.Path, "/lapi/live/getH5Play/42"):
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			// 自动画质参数会让斗鱼忽略所选清晰度。
			for _, key := range []string{"iar", "ive", "hevc", "fa"} {
				if req.Form.Has(key) {
					t.Fatalf("取流请求不应包含 %s", key)
				}
			}
			rate := req.Form.Get("rate")
			rates = append(rates, rate)
			if rate == "3" && req.Form.Get("cdn") != "line1" {
				t.Fatalf("切换清晰度丢失当前线路：%q", req.Form.Get("cdn"))
			}
			body = fmt.Sprintf(`{"error":0,"data":{"cdn":"line1","rtmp_url":"https://cdn.example/live","rtmp_live":"stream_%s.flv","rate":%s,"multirates":[{"rate":0,"name":"原画"},{"rate":4,"name":"蓝光4M"},{"rate":3,"name":"超清"},{"rate":2,"name":"高清"}],"cdnsWithName":[{"cdn":"line1","name":"线路1"}]}}`, rate, rate)
		default:
			t.Fatalf("意外请求 %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})

	s, err := c.Resolve(context.Background(), Douyu, "42", 4)
	if err != nil {
		t.Fatal(err)
	}
	if s.QualityID != 4 || s.URL != "https://cdn.example/live/stream_4.flv" || len(s.Qualities) != 4 || s.Qualities[0].Name != "原画" {
		t.Fatalf("清晰度取流错误：%+v", s)
	}
	if s.SourceID != "line1" || len(s.Sources) != 1 || s.Sources[0].URL != s.URL {
		t.Fatalf("线路信息错误：%+v", s.Sources)
	}
	if _, err = c.Resolve(context.Background(), Douyu, "42", 3, "line1"); err != nil {
		t.Fatal(err)
	}
	if len(rates) != 2 || rates[0] != "4" || rates[1] != "3" {
		t.Fatalf("切换清晰度未使用所选 rate：%v", rates)
	}
}
