package library

import (
	"encoding/json"
	"errors"
	"strings"

	"live-mygo/internal/platforms"
)

// 备份的类型中没有账号凭据、请求头或观看历史。
func (s *State) Backup() ([]byte, error) {
	return json.MarshalIndent(struct {
		Favorites   []platforms.Room           `json:"favorites"`
		Details     map[string]FavoriteDetails `json:"favorite_details,omitempty"`
		Preferences Preferences                `json:"preferences"`
	}{s.Favorites, s.FavoriteDetails, s.Preferences}, "", "  ")
}

// 合并关注，不清空当前收藏；偏好只修改备份里明确包含的字段。
func (s *State) ImportBackup(data []byte) error {
	var payload struct {
		Favorites   []platforms.Room           `json:"favorites"`
		Details     map[string]FavoriteDetails `json:"favorite_details"`
		Preferences json.RawMessage            `json:"preferences"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return errors.New("无法读取这个备份文件")
	}
	if len(payload.Favorites) == 0 && len(payload.Preferences) == 0 {
		return errors.New("文件中没有关注或偏好设置")
	}
	previous := *s
	next := *s
	next.Preferences.BlockedKeywords = append([]string(nil), s.Preferences.BlockedKeywords...)
	next.Preferences.BlockedUsers = append([]BlockedUser(nil), s.Preferences.BlockedUsers...)
	next.Favorites = append([]platforms.Room(nil), s.Favorites...)
	next.FavoriteDetails = make(map[string]FavoriteDetails, len(s.FavoriteDetails)+len(payload.Details))
	for key, details := range s.FavoriteDetails {
		next.FavoriteDetails[key] = details
	}
	if len(payload.Preferences) > 0 {
		if err := json.Unmarshal(payload.Preferences, &next.Preferences); err != nil {
			return errors.New("备份中的偏好设置无效")
		}
	}
	next.Preferences.Volume = max(0, min(100, next.Preferences.Volume))
	next.Preferences.DefaultQuality = platforms.NormalizeQualityPreference(next.Preferences.DefaultQuality)
	for _, room := range payload.Favorites {
		if room.ID == "" || strings.ContainsAny(room.ID, "\r\n\x00") {
			return errors.New("备份包含无效的直播间")
		}
		switch room.Platform {
		case platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin, platforms.Qie, platforms.FLive:
		default:
			return errors.New("备份包含无法识别的来源")
		}
		if !next.Has(room) {
			next.Favorites = append(next.Favorites, room)
		}
		if details, ok := payload.Details[FavoriteKey(room)]; ok {
			var err error
			details, err = normalizeFavoriteDetails(details)
			if err != nil {
				return err
			}
			next.FavoriteDetails[FavoriteKey(room)] = details
		}
	}
	*s = next
	if err := s.save(); err != nil {
		*s = previous
		return err
	}
	return nil
}
