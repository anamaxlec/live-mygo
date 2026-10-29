package platforms

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRoomLinks(t *testing.T) {
	for _, tc := range []struct {
		p      Platform
		input  string
		wantP  Platform
		wantID string
	}{
		{Bilibili, "https://www.douyu.com/24422?dyshid=sample", Douyu, "24422"},
		{Douyu, "https://live.bilibili.com/6?from=search", Bilibili, "6"},
		{Bilibili, " 7734200 ", Bilibili, "7734200"},
	} {
		p, id, err := ParseRoom(tc.p, tc.input)
		if err != nil || p != tc.wantP || id != tc.wantID {
			t.Fatalf("解析 %q：%s %s %v", tc.input, p, id, err)
		}
	}
	for _, input := range []string{"https://live.bilibili.com.evil.example/6", "https://www.douyu.com@evil.example/6", "file:///etc/passwd", "https://live.bilibili.com/6/extra", "0", "6\nCookie: bad"} {
		if _, _, err := ParseRoom(Bilibili, input); err == nil {
			t.Fatalf("接受了无效输入：%q", input)
		}
	}
}

func TestBilibiliUsesActualQualityAndAvcStream(t *testing.T) {
	fixture := `{"data":{"playurl_info":{"playurl":{"g_qn_desc":[{"qn":10000,"desc":"原画"},{"qn":250,"desc":"超清"}],"stream":[{"format":[{"format_name":"flv","codec":[{"codec_name":"hevc","current_qn":10000,"base_url":"/hevc.flv","url_info":[{"host":"https://cdn.example","extra":"?token=x"}]},{"codec_name":"avc","current_qn":250,"accept_qn":[10000,250],"base_url":"/avc.flv","url_info":[{"host":"https://mcdn.example","extra":"?token=x"},{"host":"https://cdn.example","extra":"?token=x"}]}]}]}]}}}}`
	var response biliPlay
	if err := json.Unmarshal([]byte(fixture), &response); err != nil {
		t.Fatal(err)
	}
	s, err := parseBiliPlay(response)
	if err != nil {
		t.Fatal(err)
	}
	if s.QualityID != 250 || s.URL != "https://cdn.example/avc.flv?token=x" || len(s.Qualities) != 2 {
		t.Fatalf("错误取流结果：%+v", s)
	}
}

func TestDouyuSigningUsesMD5AndCanCancel(t *testing.T) {
	got, err := signDouyu(context.Background(), `function ub98484234(rid,did,t){return 'rid='+rid+'&sign='+CryptoJS.MD5('abc').toString();}`, "24422", "guest", 1)
	if err != nil || got != "rid=24422&sign=900150983cd24fb0d6963f7d28e17f72" {
		t.Fatalf("签名结果错误：%q %v", got, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err = signDouyu(ctx, `while(true){}`, "1", "guest", 1); err == nil {
		t.Fatal("无限脚本未被取消")
	}
	if time.Since(start) > time.Second {
		t.Fatal("签名取消耗时过长")
	}
}

type testCookies struct{}

func (testCookies) Header(Platform, string) (string, error) { return "session=private", nil }

func TestAPIRedirectDoesNotLeakCookie(t *testing.T) {
	var received string
	dst := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received = r.Header.Get("Cookie"); w.Write([]byte(`{}`)) }))
	defer dst.Close()
	// hostname 不同，明确触发跨域重定向策略。
	target := "http://localhost" + dst.URL[len("http://127.0.0.1"):]
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") == "" {
			t.Error("初始接口缺少测试 Cookie")
		}
		http.Redirect(w, r, target, http.StatusFound)
	}))
	defer src.Close()
	c := NewClient(testCookies{})
	var out map[string]any
	if err := c.request(context.Background(), Bilibili, http.MethodGet, src.URL, "https://live.bilibili.com/", nil, &out); err != nil {
		t.Fatal(err)
	}
	if received != "" {
		t.Fatal("跨主机重定向发送了账号 Cookie")
	}
}

func TestBilibiliSourcesStayAtSameQualityAndHideTokens(t *testing.T) {
	fixture := `{"data":{"playurl_info":{"playurl":{"stream":[{"format":[{"format_name":"flv","codec":[{"codec_name":"avc","current_qn":250,"base_url":"/x.flv","url_info":[{"host":"https://a.example","extra":"?token=private"},{"host":"https://b.example","extra":"?token=private"}]}]},{"format_name":"ts","codec":[{"codec_name":"avc","current_qn":250,"base_url":"/x.m3u8","url_info":[{"host":"https://a.example","extra":"?token=private"}]},{"codec_name":"avc","current_qn":80,"base_url":"/low.m3u8","url_info":[{"host":"https://c.example","extra":"?token=private"}]}]}]}]}}}}`
	var play biliPlay
	if err := json.Unmarshal([]byte(fixture), &play); err != nil {
		t.Fatal(err)
	}
	s, err := parseBiliPlay(play)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sources) != 3 {
		t.Fatalf("应保留当前画质的 3 条线路，得到 %+v", s.Sources)
	}
	if !s.SelectSource("a.example|ts") || s.Format != "HLS" || s.QualityID != 250 {
		t.Fatalf("没有选择 HLS 或切换了画质：%+v", s)
	}
	data, _ := json.Marshal(s)
	if bytes.Contains(data, []byte("private")) || bytes.Contains(data, []byte("https://")) {
		t.Fatal("序列化泄露播放令牌")
	}
}
