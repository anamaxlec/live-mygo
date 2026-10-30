package library

import (
	"os"
	"path/filepath"
	"testing"

	"live-mygo/internal/platforms"
)

func TestPersistenceAndPlatformIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	bili := platforms.Room{Platform: platforms.Bilibili, ID: "42", Title: "first"}
	douyu := platforms.Room{Platform: platforms.Douyu, ID: "42", Title: "second"}
	for _, room := range []platforms.Room{bili, douyu} {
		if err = s.Toggle(room); err != nil {
			t.Fatal(err)
		}
		if err = s.Watch(room); err != nil {
			t.Fatal(err)
		}
	}
	bili.Title = "updated"
	if err = s.Watch(bili); err != nil {
		t.Fatal(err)
	}
	if err = s.SetPreferences(Preferences{Appearance: "dark", Volume: 38, PauseWhenBrowsing: true}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Has(bili) || !reloaded.Has(douyu) || len(reloaded.Favorites) != 2 {
		t.Fatal("两个平台相同的房间号不应互相覆盖")
	}
	if len(reloaded.History) != 2 || reloaded.History[0].Title != "updated" || reloaded.History[1].Platform != platforms.Douyu {
		t.Fatal("历史应去重并移动到最前")
	}
	if reloaded.Preferences.Appearance != "dark" || reloaded.Preferences.Volume != 38 || !reloaded.Preferences.PauseWhenBrowsing {
		t.Fatal("偏好未能持久化")
	}
}

func TestDefaultQualityDefaultsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Preferences.DefaultQuality != platforms.QualityHighest {
		t.Fatalf("默认画质应为最高画质，实际 %q", s.Preferences.DefaultQuality)
	}
	if err = s.SetPreferences(Preferences{DefaultQuality: platforms.QualityUltra}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Preferences.DefaultQuality != platforms.QualityUltra {
		t.Fatalf("默认画质未持久化：%q", reloaded.Preferences.DefaultQuality)
	}
	if err = reloaded.SetPreferences(Preferences{DefaultQuality: "bogus"}); err != nil {
		t.Fatal(err)
	}
	if reloaded.Preferences.DefaultQuality != platforms.QualityHighest {
		t.Fatalf("无效默认画质应归一为最高画质：%q", reloaded.Preferences.DefaultQuality)
	}
}

func TestLegacyPreferencesGainDefaultQuality(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte(`{"preferences":{"appearance":"dark","volume":30}}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Preferences.DefaultQuality != platforms.QualityHighest {
		t.Fatalf("旧配置应获得最高画质默认值，实际 %q", s.Preferences.DefaultQuality)
	}
}

func TestUpdateFavoritesPersistsLiveStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	douyu := platforms.Room{Platform: platforms.Douyu, ID: "6657", Title: "旧标题", Anchor: "主播"}
	qie := platforms.Room{Platform: platforms.Qie, ID: "1", Title: "企鹅"}
	for _, r := range []platforms.Room{douyu, qie} {
		if err = s.Toggle(r); err != nil {
			t.Fatal(err)
		}
	}
	fresh := platforms.Room{Platform: platforms.Douyu, ID: "6657", Title: "新标题", Anchor: "主播", Live: true, Cover: "https://example.com/cover.jpg"}
	if err = s.UpdateFavorites([]platforms.Room{fresh}); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Favorites) != 2 || reloaded.Favorites[0].ID != "1" || reloaded.Favorites[1].ID != "6657" {
		t.Fatalf("收藏顺序不应被改变：%+v", reloaded.Favorites)
	}
	updated := reloaded.Favorites[1]
	if updated.Title != "新标题" || !updated.Live || updated.Cover != "https://example.com/cover.jpg" {
		t.Fatalf("关注状态未更新：%+v", updated)
	}
}

func TestFailedWriteDoesNotChangeVisibleState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "blocked", "library.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "blocked"), []byte("file"), 0600); err != nil {
		t.Fatal(err)
	}
	r := platforms.Room{Platform: platforms.Bilibili, ID: "6"}
	if s.Toggle(r) == nil || s.Has(r) {
		t.Fatal("保存失败不能显示为已关注")
	}
	if s.Watch(r) == nil || len(s.History) != 0 {
		t.Fatal("保存失败不能保留虚假的历史状态")
	}
	if s.SetPreferences(Preferences{Appearance: "dark", Volume: 1}) == nil || s.Preferences.Appearance != "system" {
		t.Fatal("保存失败应保留原有偏好")
	}
}
