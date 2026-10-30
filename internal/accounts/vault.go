package accounts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"live-mygo/internal/platforms"
)

type Secrets interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
}
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"http_only"`
}
type savedAccount struct {
	Cookies []Cookie  `json:"cookies"`
	SavedAt time.Time `json:"saved_at"`
}
type Vault struct {
	Dir     string
	Secrets Secrets
	mu      sync.Mutex
}

func validPlatform(p platforms.Platform) bool {
	return p == platforms.Bilibili || p == platforms.Douyu || p == platforms.Huya || p == platforms.Douyin || p == platforms.Qie
}
func allowed(p platforms.Platform, domain string) bool {
	h := strings.TrimPrefix(strings.ToLower(domain), ".")
	root := p.Domain()
	return root != "" && (h == root || strings.HasSuffix(h, "."+root))
}

func Filter(p platforms.Platform, cookies []Cookie) []Cookie {
	var out []Cookie
	for _, c := range cookies {
		if !allowed(p, c.Domain) || c.Name == "" || !c.Expires.IsZero() && c.Expires.Before(time.Now()) {
			continue
		}
		if strings.ContainsAny(c.Name+c.Value, "\r\n\x00") {
			continue
		}
		out = append(out, c)
	}
	return out
}

func HasLogin(p platforms.Platform, cookies []Cookie) bool {
	wanted := "SESSDATA"
	if p == platforms.Douyu {
		wanted = "acf_auth"
	}
	if p == platforms.Douyin {
		wanted = "sessionid"
	}
	if p == platforms.Qie {
		wanted = "token"
	}
	for _, c := range Filter(p, cookies) {
		if c.Name == wanted && c.Value != "" {
			return true
		}
	}
	return false
}

func (v *Vault) Save(p platforms.Platform, cookies []Cookie) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !validPlatform(p) {
		return errors.New("不支持的账号平台")
	}
	cookies = Filter(p, cookies)
	if !HasLogin(p, cookies) {
		return fmt.Errorf("尚未检测到%s登录 Cookie，请先在官方页面完成登录", p.Name())
	}
	key, err := v.Secrets.Get("cookie-encryption-key")
	if err != nil {
		return fmt.Errorf("读取系统凭据存储失败：%w", err)
	}
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
		if err = v.Secrets.Set("cookie-encryption-key", key); err != nil {
			return fmt.Errorf("保存系统凭据失败：%w", err)
		}
	}
	aead, err := newAEAD(key)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(savedAccount{cookies, time.Now()})
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	sealed := aead.Seal(nonce, nonce, plain, []byte(p))
	if err = os.MkdirAll(v.Dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(v.Dir, ".cookie-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err = f.Write(sealed); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), v.path(p))
}

func (v *Vault) Load(p platforms.Platform) ([]Cookie, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !validPlatform(p) {
		return nil, errors.New("不支持的账号平台")
	}
	data, err := os.ReadFile(v.path(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	key, err := v.Secrets.Get("cookie-encryption-key")
	if err != nil {
		return nil, fmt.Errorf("读取系统凭据失败：%w", err)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(data) < aead.NonceSize() {
		return nil, errors.New("本地账号文件不完整")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(p))
	if err != nil {
		return nil, errors.New("本地账号解密失败")
	}
	var saved savedAccount
	if err = json.Unmarshal(plain, &saved); err != nil {
		return nil, errors.New("本地账号文件格式错误")
	}
	return Filter(p, saved.Cookies), nil
}

func (v *Vault) Remove(p platforms.Platform) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !validPlatform(p) {
		return errors.New("不支持的账号平台")
	}
	err := os.Remove(v.path(p))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (v *Vault) Header(p platforms.Platform, target string) (string, error) {
	cookies, err := v.Load(p)
	if err != nil {
		return "", err
	}
	return Header(p, cookies, target)
}

func Header(p platforms.Platform, cookies []Cookie, target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("无效的 Cookie 请求地址")
	}
	// CDN 使用播放地址自带的签名；账号 Cookie 只发往账号所属站点。
	if !allowed(p, u.Hostname()) {
		return "", nil
	}
	jar, _ := cookiejar.New(nil)
	for _, c := range Filter(p, cookies) {
		host := strings.TrimPrefix(c.Domain, ".")
		origin := &url.URL{Scheme: "https", Host: host, Path: "/"}
		jar.SetCookies(origin, []*http.Cookie{{Name: c.Name, Value: c.Value, Domain: c.Domain, Path: c.Path, Expires: c.Expires, Secure: c.Secure, HttpOnly: c.HTTPOnly}})
	}
	req := &http.Request{Header: make(http.Header)}
	for _, c := range jar.Cookies(u) {
		req.AddCookie(c)
	}
	return req.Header.Get("Cookie"), nil
}

func (v *Vault) path(p platforms.Platform) string { return filepath.Join(v.Dir, string(p)+".enc") }
func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("系统凭据中没有有效的账号加密密钥")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
