package platforms

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDouyuCompatibilityIsOptInAndPreservesCertificateChecks(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" {
			t.Error("公开弹幕兼容连接发送了 Cookie")
		}
		w.Write([]byte("ok"))
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS12, MaxVersion: tls.VersionTLS12, CipherSuites: []uint16{tls.TLS_RSA_WITH_AES_128_GCM_SHA256}}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	c := NewClient(nil)
	c.HTTP = server.Client()
	c.HTTP.Jar, _ = cookiejar.New(nil)
	u, _ := url.Parse(server.URL)
	c.HTTP.Jar.SetCookies(u, []*http.Cookie{{Name: "test_session", Value: "synthetic"}})
	if c.douyuChatClient() != c.HTTP {
		t.Fatal("默认不应启用兼容连接")
	}
	c.SetDouyuChatCompatibility(true)
	compatible := c.douyuChatClient()
	defer compatible.CloseIdleConnections()
	cfg := compatible.Transport.(*http.Transport).TLSClientConfig
	if cfg.InsecureSkipVerify || cfg.MinVersion < tls.VersionTLS12 {
		t.Fatal("证书校验或 TLS 下限被削弱")
	}
	resp, err := compatible.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	// 证书不受信任时，即使已启用兼容，也必须失败。
	untrusted := NewClient(nil)
	untrusted.SetDouyuChatCompatibility(true)
	bad := untrusted.douyuChatClient()
	defer bad.CloseIdleConnections()
	if resp, err = bad.Get(server.URL); err == nil {
		resp.Body.Close()
		t.Fatal("接受了不可信证书")
	}
	if c.HTTP.Transport == compatible.Transport {
		t.Fatal("修改了普通 API 使用的客户端")
	}
	c.SetDouyuChatCompatibility(false)
	if c.douyuChatClient() != c.HTTP {
		t.Fatal("关闭开关未恢复标准客户端")
	}
}

func TestDouyuCompatibilityDoesNotFollowRedirects(t *testing.T) {
	c := NewClient(nil)
	c.SetDouyuChatCompatibility(true)
	client := c.douyuChatClient()
	defer client.CloseIdleConnections()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://unrelated.example", nil)
	if client.CheckRedirect(req, nil) == nil {
		t.Fatal("兼容连接不应被重定向到其他主机")
	}
}
