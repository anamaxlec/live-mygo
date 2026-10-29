package accounts

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"live-mygo/internal/platforms"
)

type memorySecrets struct{ key []byte }

func (s *memorySecrets) Get(string) ([]byte, error) { return s.key, nil }
func (s *memorySecrets) Set(_ string, key []byte) error {
	s.key = append([]byte(nil), key...)
	return nil
}

func TestVaultEncryptsAndRejectsTampering(t *testing.T) {
	v := Vault{Dir: t.TempDir(), Secrets: &memorySecrets{}}
	cs := []Cookie{{Name: "SESSDATA", Value: "test-private-cookie", Domain: ".bilibili.com", Path: "/", Secure: true, HTTPOnly: true}}
	if err := v.Save(platforms.Bilibili, cs); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(v.Dir, "bilibili.enc")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("test-private-cookie")) {
		t.Fatal("明文 Cookie 被写入磁盘")
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("账号文件权限不是 0600")
	}
	got, err := v.Header(platforms.Bilibili, "https://api.live.bilibili.com/room")
	if err != nil || got != "SESSDATA=test-private-cookie" {
		t.Fatalf("读取失败：%q %v", got, err)
	}
	raw[len(raw)-1] ^= 1
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = v.Load(platforms.Bilibili); err == nil {
		t.Fatal("接受了被改动的密文")
	}
}

func TestCookieScopeAndExpiry(t *testing.T) {
	cs := []Cookie{
		{Name: "SESSDATA", Value: "valid", Domain: ".bilibili.com", Path: "/", Secure: true},
		{Name: "expired", Value: "old", Domain: ".bilibili.com", Path: "/", Expires: time.Now().Add(-time.Hour)},
		{Name: "foreign", Value: "wrong", Domain: ".douyu.com", Path: "/"},
		{Name: "hostonly", Value: "one", Domain: "passport.bilibili.com", Path: "/"},
	}
	for _, u := range []string{"https://cdn.bilivideo.com/live.flv", "https://bilibili.com.evil.example/", "https://www.douyu.com/", "http://api.live.bilibili.com/"} {
		got, err := Header(platforms.Bilibili, cs, u)
		if err != nil || got != "" {
			t.Fatalf("Cookie 越界：%s %q %v", u, got, err)
		}
	}
	got, err := Header(platforms.Bilibili, cs, "https://api.live.bilibili.com/")
	if err != nil || got != "SESSDATA=valid" || strings.Contains(got, "hostonly") {
		t.Fatalf("Cookie 域或过期处理错误：%q %v", got, err)
	}
}

func TestSaveRequiresOwnPlatformLoginCookie(t *testing.T) {
	v := Vault{Dir: t.TempDir(), Secrets: &memorySecrets{}}
	if err := v.Save(platforms.Douyu, []Cookie{{Name: "acf_auth", Value: "x", Domain: ".notdouyu.com", Path: "/"}}); err == nil {
		t.Fatal("接受了其他站点的 Cookie")
	}
	if err := v.Save(platforms.Bilibili, []Cookie{{Name: "buvid3", Value: "guest", Domain: ".bilibili.com", Path: "/"}}); err == nil {
		t.Fatal("将游客会话当成登录")
	}
}
