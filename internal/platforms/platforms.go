package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

type Platform string

const (
	Bilibili  Platform = "bilibili"
	Douyu     Platform = "douyu"
	UserAgent          = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
)

func (p Platform) Name() string {
	if p == Bilibili {
		return "哔哩哔哩"
	}
	if p == Douyu {
		return "斗鱼"
	}
	return string(p)
}

func (p Platform) Domain() string {
	if p == Bilibili {
		return "bilibili.com"
	}
	if p == Douyu {
		return "douyu.com"
	}
	return ""
}

func (p Platform) LoginURL() string {
	if p == Bilibili {
		return "https://passport.bilibili.com/login"
	}
	return "https://www.douyu.com/"
}

type Quality struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}
type Room struct {
	Platform Platform `json:"platform"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Anchor   string   `json:"anchor"`
	Live     bool     `json:"live"`
	Cover    string   `json:"cover,omitempty"`
	Category string   `json:"category,omitempty"`
	Heat     int64    `json:"heat,omitempty"`
}
type Stream struct {
	Room      Room              `json:"room"`
	Qualities []Quality         `json:"qualities"`
	QualityID int               `json:"quality_id"`
	Format    string            `json:"format"`
	URL       string            `json:"-"`
	Headers   map[string]string `json:"-"`
	Sources   []Source          `json:"sources"`
	SourceID  string            `json:"source_id"`
}

// Source 保留平台实际返回的线路；播放地址与请求头不进入报告或持久化数据。
type Source struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Format  string            `json:"format"`
	URL     string            `json:"-"`
	Headers map[string]string `json:"-"`
}

func (s *Stream) SelectSource(id string) bool {
	for _, source := range s.Sources {
		if source.ID == id && source.URL != "" {
			s.SourceID, s.URL, s.Headers, s.Format = source.ID, source.URL, source.Headers, source.Format
			return true
		}
	}
	return false
}

// SessionCookies 只返回目标 URL 可以接收的 Cookie，不向无关 CDN 发送账号凭据。
type SessionCookies interface {
	Header(Platform, string) (string, error)
}

type Client struct {
	HTTP               *http.Client
	Cookies            SessionCookies
	guestBili          string
	douyuCompatibility atomic.Bool
}

func NewClient(cookies SessionCookies) *Client {
	return &Client{HTTP: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("重定向次数过多")
		}
		// 手动 Cookie 只允许跟随同一主机，防止接口重定向泄露凭据。
		if len(via) > 0 && req.URL.Hostname() != via[0].URL.Hostname() {
			req.Header.Del("Cookie")
		}
		return nil
	}}, Cookies: cookies}
}

var numeric = regexp.MustCompile(`^[1-9][0-9]{0,15}$`)
var roomAlias = regexp.MustCompile(`^[A-Za-z0-9_]{1,40}$`)

func ParseRoom(p Platform, input string) (Platform, string, error) {
	input = strings.TrimSpace(input)
	if strings.Contains(input, "://") {
		u, err := url.Parse(input)
		if err != nil || u.Scheme != "https" && u.Scheme != "http" || u.User != nil {
			return p, "", errors.New("请输入有效的直播间链接")
		}
		host := strings.ToLower(u.Hostname())
		switch {
		case host == "live.bilibili.com":
			p = Bilibili
		case host == "douyu.com" || strings.HasSuffix(host, ".douyu.com"):
			p = Douyu
		default:
			return p, "", errors.New("只支持 B 站和斗鱼的官方直播间链接")
		}
		input = strings.Trim(u.Path, "/")
		if strings.Contains(input, "/") {
			return p, "", errors.New("请使用直播间的直接链接")
		}
	}
	if p != Bilibili && p != Douyu {
		return p, "", errors.New("不支持的平台")
	}
	if p == Bilibili && !numeric.MatchString(input) || p == Douyu && !roomAlias.MatchString(input) {
		return p, "", errors.New("请输入房间号或直播间链接")
	}
	return p, input, nil
}

func (c *Client) Resolve(ctx context.Context, p Platform, id string, quality int, route ...string) (*Stream, error) {
	p, id, err := ParseRoom(p, id)
	if err != nil {
		return nil, err
	}
	if p == Bilibili {
		return c.bilibili(ctx, id, quality, route...)
	}
	return c.douyu(ctx, id, quality, route...)
}

func (c *Client) request(ctx context.Context, p Platform, method, target, referer string, form url.Values, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Referer", referer)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if c.Cookies != nil {
		cookie, e := c.Cookies.Header(p, target)
		if e != nil {
			return fmt.Errorf("读取本地账号失败：%w", e)
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
	}
	if p == Bilibili && c.guestBili != "" && !strings.Contains(req.Header.Get("Cookie"), "buvid3=") {
		cookie := req.Header.Get("Cookie")
		if cookie != "" {
			cookie += "; "
		}
		req.Header.Set("Cookie", cookie+c.guestBili)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s网络请求失败", p.Name())
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s接口返回 HTTP %d", p.Name(), resp.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
		return fmt.Errorf("%s返回了无法解析的数据", p.Name())
	}
	return nil
}

func (c *Client) mediaHeaders(p Platform, target, referer string) (map[string]string, error) {
	h := map[string]string{"User-Agent": UserAgent, "Referer": referer}
	if c.Cookies != nil {
		v, err := c.Cookies.Header(p, target)
		if err != nil {
			return nil, err
		}
		if v != "" {
			h["Cookie"] = v
		}
	}
	return h, nil
}
