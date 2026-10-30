package library

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"live-mygo/internal/platforms"
)

type State struct {
	Favorites   []platforms.Room `json:"favorites"`
	History     []platforms.Room `json:"history"`
	Preferences Preferences      `json:"preferences"`
	path        string
}

type Preferences struct {
	StartupPage       string        `json:"startup_page"`
	RaceMenu          bool          `json:"race_menu,omitempty"`
	ChatWidth         float64       `json:"chat_width"`
	Appearance        string        `json:"appearance"`
	Volume            float64       `json:"volume"`
	DefaultQuality    string        `json:"default_quality"`
	PauseWhenBrowsing bool          `json:"pause_when_browsing"`
	Danmaku           bool          `json:"danmaku"`
	DanmakuSize       float64       `json:"danmaku_size"`
	DanmakuOpacity    float64       `json:"danmaku_opacity"`
	FollowVideo       bool          `json:"follow_video"`
	ChatPanel         bool          `json:"chat_panel"`
	ChatSize          float64       `json:"chat_size"`
	ChatTime          bool          `json:"chat_time"`
	ChatCompact       bool          `json:"chat_compact"`
	DouyuLegacyTLS    bool          `json:"douyu_legacy_tls"`
	ChatBadges        bool          `json:"chat_badges"`
	ChatConnection    bool          `json:"chat_connection"`
	FilterEnabled     bool          `json:"filter_enabled"`
	FilterDuplicates  bool          `json:"filter_duplicates"`
	BlockedKeywords   []string      `json:"blocked_keywords,omitempty"`
	BlockedUsers      []BlockedUser `json:"blocked_users,omitempty"`
	DanmakuSpeed      float64       `json:"danmaku_speed"`
	DanmakuFont       string        `json:"danmaku_font"`
	DanmakuWeight     int           `json:"danmaku_weight"`
	DanmakuStroke     float64       `json:"danmaku_stroke"`
	DanmakuLines      int           `json:"danmaku_lines"`
	DanmakuGap        float64       `json:"danmaku_gap"`
	DanmakuTop        float64       `json:"danmaku_top"`
	DanmakuBottom     float64       `json:"danmaku_bottom"`
	DanmakuFPS        int           `json:"danmaku_fps"`
	DanmakuMerge      bool          `json:"danmaku_merge"`
	DanmakuColor      string        `json:"danmaku_color"`
	DanmakuArea       float64       `json:"danmaku_area"`
}

type BlockedUser struct {
	Platform platforms.Platform `json:"platform"`
	ID       string             `json:"id"`
	Name     string             `json:"name"`
}

func Load(path string) (*State, error) {
	s := &State{path: path, Preferences: Preferences{Appearance: "system", Volume: 60, DefaultQuality: platforms.QualityHighest, Danmaku: true, DanmakuMerge: true, DanmakuSize: 24, DanmakuOpacity: 80, ChatPanel: true, ChatSize: 13, ChatBadges: true, ChatConnection: true, FilterEnabled: true, DanmakuSpeed: 1, DanmakuArea: .35, DanmakuWeight: 500, DanmakuStroke: 1, DanmakuLines: 4, DanmakuGap: 10, DanmakuTop: 12, DanmakuBottom: 24, DanmakuFPS: 60, DanmakuColor: "平台颜色"}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, s); err == nil {
		s.Preferences.DefaultQuality = platforms.NormalizeQualityPreference(s.Preferences.DefaultQuality)
	}
	return s, err
}
func (s *State) Has(r platforms.Room) bool {
	for _, v := range s.Favorites {
		if same(v, r) {
			return true
		}
	}
	return false
}
func (s *State) Toggle(r platforms.Room) error {
	previous := s.Favorites
	next := make([]platforms.Room, 0, len(previous)+1)
	found := false
	for i, v := range s.Favorites {
		if same(v, r) {
			next = append(next, previous[:i]...)
			next = append(next, previous[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		next = append(next, r)
		next = append(next, previous...)
	}
	s.Favorites = next
	if err := s.save(); err != nil {
		s.Favorites = previous
		return err
	}
	return nil
}

// UpdateFavorites 刷新收藏房间的标题、封面和开播状态，保留收藏顺序与本地 ID。
func (s *State) UpdateFavorites(fresh []platforms.Room) error {
	if len(fresh) == 0 {
		return nil
	}
	byKey := make(map[string]platforms.Room, len(fresh))
	for _, r := range fresh {
		byKey[string(r.Platform)+":"+r.ID] = r
	}
	next := append([]platforms.Room(nil), s.Favorites...)
	changed := false
	for i, old := range next {
		r, ok := byKey[string(old.Platform)+":"+old.ID]
		if !ok {
			continue
		}
		r.Platform, r.ID = old.Platform, old.ID
		if r.Anchor == "" {
			r.Anchor = old.Anchor
		}
		if r.Cover == "" {
			r.Cover = old.Cover
		}
		if r != old {
			next[i] = r
			changed = true
		}
	}
	if !changed {
		return nil
	}
	previous := s.Favorites
	s.Favorites = next
	if err := s.save(); err != nil {
		s.Favorites = previous
		return err
	}
	return nil
}

func (s *State) Watch(r platforms.Room) error {
	next := []platforms.Room{r}
	for _, v := range s.History {
		if !same(v, r) && len(next) < 100 {
			next = append(next, v)
		}
	}
	previous := s.History
	s.History = next
	if err := s.save(); err != nil {
		s.History = previous
		return err
	}
	return nil
}

func (s *State) SetPreferences(next Preferences) error {
	next.Volume = max(0, min(100, next.Volume))
	next.DefaultQuality = platforms.NormalizeQualityPreference(next.DefaultQuality)
	previous := s.Preferences
	s.Preferences = next
	if err := s.save(); err != nil {
		s.Preferences = previous
		return err
	}
	return nil
}
func same(a, b platforms.Room) bool { return a.Platform == b.Platform && a.ID == b.ID }
func (s *State) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".library-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
