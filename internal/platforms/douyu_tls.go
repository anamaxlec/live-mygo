package platforms

import (
	"crypto/tls"
	"errors"
	"net/http"
	"strings"
)

// 斗鱼公开弹幕服务当前仅协商旧式 RSA AES-GCM。必须经用户启用，
// 不发送 Cookie，保留 TLS 1.2 下限和系统证书验证；其他请求使用原客户端。
func (c *Client) SetDouyuChatCompatibility(enabled bool) { c.douyuCompatibility.Store(enabled) }
func (c *Client) douyuChatClient() *http.Client {
	if !c.douyuCompatibility.Load() {
		return c.HTTP
	}
	original := c.HTTP.Transport
	if original == nil {
		original = http.DefaultTransport
	}
	base, ok := original.(*http.Transport)
	if !ok {
		return c.HTTP
	}
	transport := base.Clone()
	cfg := transport.TLSClientConfig
	if cfg == nil {
		cfg = &tls.Config{}
	} else {
		cfg = cfg.Clone()
	}
	cfg.MinVersion = max(cfg.MinVersion, tls.VersionTLS12)
	// 不继承任何跳过校验的调试配置。
	cfg.InsecureSkipVerify = false
	if cfg.CipherSuites == nil {
		for _, suite := range tls.CipherSuites() {
			if strings.Contains(suite.Name, "GCM") || strings.Contains(suite.Name, "CHACHA20") {
				cfg.CipherSuites = append(cfg.CipherSuites, suite.ID)
			}
		}
	}
	cfg.CipherSuites = append(cfg.CipherSuites, tls.TLS_RSA_WITH_AES_128_GCM_SHA256)
	transport.TLSClientConfig = cfg
	client := *c.HTTP
	client.Transport = transport
	client.Jar = nil
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return errors.New("弹幕连接不接受重定向")
	}
	return &client
}
