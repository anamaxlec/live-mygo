package library

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"live-mygo/internal/platforms"
)

func TestImportSimpleLiveProfileMergesFavoritesAndSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	room := platforms.Room{Platform: platforms.Douyu, ID: "2"}
	if err := s.Toggle(room); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFavoriteDetails(room, FavoriteDetails{Alias: "我的别名", Group: "本地"}); err != nil {
		t.Fatal(err)
	}
	s.Preferences.BlockedKeywords = []string{" Local "}
	data := []byte(`{
		"schema":"simple_live_profile","schemaVersion":4,"platform":"android",
		"settings":{"ThemeMode":2,"PlayerVolume":25.0,"DanmuEnable":false,"DanmuOpacity":0.55,
			"DanmuDedupeEnable":true,"BilibiliCookie":"secret","ChatPanel":false},
		"danmuShield":{"raw":["keyword:测试","keyword:local","user:bilibili:123"]},
		"followUsers":[
			{"id":"douyu_2","siteId":"douyu","roomId":"2","isSpecialFollow":true,"tag":"外部"},
			{"id":"douyin_123","siteId":"douyin","roomId":90071992547409931,"userName":"主播"},
			{"siteId":"twitch","roomId":"third"},
			{"siteId":"huya","roomId":"https://example.com"}
		],
		"followUserTags":[{"id":"1","tag":"赛事","userId":["douyin_123"]}],
		"accounts":{"items":[{"cookie":"secret"}]},
		"histories":[{"siteId":"huya","roomId":"999"}]
	}`)
	report, err := s.ImportBackupWithReport(data)
	if want := (ImportReport{Format: BackupSimple, Added: 1, Existing: 1, Skipped: 2, Settings: 6}); err != nil || report != want {
		t.Fatalf("report = %+v, err = %v, want %+v", report, err, want)
	}
	s, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Preferences
	if p.Appearance != "dark" || p.Volume != 25 || p.Danmaku || !p.ChatPanel || !p.FilterDuplicates ||
		p.DanmakuOpacity < 54.999 || p.DanmakuOpacity > 55.001 ||
		!slices.Equal(p.BlockedKeywords, []string{"Local", "测试"}) || len(s.History) != 0 {
		t.Fatalf("unexpected preferences: %+v", p)
	}
	if want := (FavoriteDetails{Alias: "我的别名", Group: "本地", Pinned: true}); s.Details(room) != want {
		t.Fatalf("existing room details = %+v, want %+v", s.Details(room), want)
	}
	if len(s.Favorites) != 2 || s.Favorites[1].ID != "90071992547409931" || s.Details(s.Favorites[1]).Group != "赛事" {
		t.Fatalf("favorites = %+v, details = %+v", s.Favorites, s.FavoriteDetails)
	}
}

func TestImportLegacySimpleAndPureLive(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		data   string
		format BackupFormat
	}{
		{`{"type":"simple_live","version":1,"config":{"ThemeMode":1,"DanmuShieldEnable":false},"shield":{"广告":"广告"}}`, BackupSimpleSettings},
		{`[{"id":"huya_123","siteId":"huya","roomId":"123","userName":"主播","tag":"全部","isSpecialFollow":1}]`, BackupSimpleFollows},
		{`{"favorites":{"data":[{"siteId":"bilibili","roomId":"456","tag":"游戏"}]}}`, BackupSimpleFollows},
		{`{"themeMode":"Dark","defaultDesktopVolume":0.75,"favoriteRooms":"{\"list\":[{\"platform\":\"bilibili\",\"roomId\":\"42\",\"nick\":\"主播\",\"tagIds\":[\"f1\"]}]}","custom_tags_data":{"tags":[{"id":"f1","name":"兴趣"}]}}`, BackupPure},
		{`{"backupVersion":4,"danmaku":{"hideDanmaku":false,"enableDanmakuDisplay":false,"enableDanmakuStroke":false,"danmakuFontSize":27,"danmakuFontBorder":3},"favorite":{"favoriteRooms":[{"platform":"douyu","roomId":"999"}],"shieldList":["重复"]},"tags":{"tags":[{"id":"t2","name":"新闻"}],"roomTagsMap":{"douyu:999":["t2"]}}}`, BackupPure},
	} {
		report, err := s.ImportBackupWithReport([]byte(input.data))
		if err != nil || report.Format != input.format {
			t.Fatalf("import %s: report = %+v, err = %v", input.format, report, err)
		}
	}
	p := s.Preferences
	if p.Appearance != "dark" || p.Volume != 75 || p.FilterEnabled || p.Danmaku || p.DanmakuStroke != 0 ||
		p.DanmakuSize != 27 || !slices.Equal(p.BlockedKeywords, []string{"广告", "重复"}) {
		t.Fatalf("unexpected preferences: %+v", p)
	}
	if len(s.Favorites) != 4 || !s.Details(s.Favorites[0]).Pinned ||
		s.Details(s.Favorites[1]).Group != "游戏" || s.Details(s.Favorites[2]).Group != "兴趣" ||
		s.Details(s.Favorites[3]).Group != "新闻" {
		t.Fatalf("favorites = %+v, details = %+v", s.Favorites, s.FavoriteDetails)
	}
}

func TestExportCompatibleBackups(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "library.json"))
	if err != nil {
		t.Fatal(err)
	}
	room := platforms.Room{Platform: platforms.Bilibili, ID: "123", Anchor: "主播", Title: "直播"}
	s.Favorites = []platforms.Room{room, {Platform: platforms.Qie, ID: "456"}}
	s.FavoriteDetails = map[string]FavoriteDetails{FavoriteKey(room): {Group: "游戏", Pinned: true}}
	s.Preferences.BlockedKeywords = []string{"广告"}
	s.Preferences.Appearance = "light"
	for _, tc := range []struct {
		format  BackupFormat
		checks  []string
		skipped int
		rooms   int
	}{
		{BackupNative, []string{`"preferences"`, `"qie"`}, 0, 2},
		{BackupSimple, []string{`"schemaVersion": 4`, `"PlayerVolume": 60.0`, `"DanmuSize": 24.0`, `"DanmuStrokeWidth": 1.0`, `"followUserTags"`}, 1, 1},
		{BackupSimpleSettings, []string{`"type": "simple_live"`, `"PlayerVolume": 60.0`, `"shield"`}, 0, 0},
		{BackupSimpleFollows, []string{`"id": "bilibili_123"`, `"isSpecialFollow": true`}, 1, 1},
		{BackupPure, []string{`"backupVersion": 3`, `"defaultDesktopVolume": 0.6`, `"themeMode": "Light"`, `"roomTagsMap"`}, 1, 1},
		{BackupPureFollows, []string{`"backupScope": "favorites"`, `"favoriteRooms"`}, 1, 1},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			data, skipped, err := s.BackupAs(tc.format)
			if err != nil || skipped != tc.skipped || !json.Valid(data) {
				t.Fatalf("export skipped %d, err %v, data %s", skipped, err, data)
			}
			for _, value := range tc.checks {
				if !strings.Contains(string(data), value) {
					t.Errorf("missing %q in %s", value, data)
				}
			}
			target, err := Load(filepath.Join(t.TempDir(), "library.json"))
			if err != nil {
				t.Fatal(err)
			}
			report, err := target.ImportBackupWithReport(data)
			if err != nil || report.Added != tc.rooms {
				t.Fatalf("import report = %+v, err = %v", report, err)
			}
			switch tc.format {
			case BackupNative, BackupSimple, BackupPure:
				if target.Details(room).Group != "游戏" || target.Preferences.Appearance != "light" {
					t.Fatal("lost a compatible group or setting")
				}
			}
		})
	}
}
