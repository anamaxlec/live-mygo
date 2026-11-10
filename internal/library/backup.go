package library

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
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
	_, err := s.ImportBackupWithReport(data)
	return err
}

type backupPayload struct {
	Favorites   []platforms.Room           `json:"favorites"`
	Details     map[string]FavoriteDetails `json:"favorite_details"`
	Preferences json.RawMessage            `json:"preferences"`
}

type ImportReport struct {
	Format                             BackupFormat
	Added, Existing, Skipped, Settings int
}

func (s *State) ImportBackupWithReport(data []byte) (ImportReport, error) {
	payload, report, err := decodeBackup(data)
	if err != nil {
		return report, err
	}
	err = s.mergeBackup(payload, &report)
	return report, err
}

func (s *State) mergeBackup(payload backupPayload, report *ImportReport) error {
	if len(payload.Favorites) == 0 && len(payload.Preferences) == 0 {
		return errors.New("文件中没有可导入的关注或偏好设置")
	}
	previous := *s
	next := *s
	next.Preferences.BlockedKeywords = append([]string(nil), s.Preferences.BlockedKeywords...)
	next.Preferences.BlockedUsers = append([]BlockedUser(nil), s.Preferences.BlockedUsers...)
	next.Preferences.RoomPlayback = maps.Clone(s.Preferences.RoomPlayback)
	next.Preferences.HiddenHomeRooms = maps.Clone(s.Preferences.HiddenHomeRooms)
	next.Favorites = append([]platforms.Room(nil), s.Favorites...)
	next.FavoriteDetails = make(map[string]FavoriteDetails, len(s.FavoriteDetails)+len(payload.Details))
	for key, details := range s.FavoriteDetails {
		next.FavoriteDetails[key] = details
	}
	if len(payload.Preferences) > 0 {
		if err := json.Unmarshal(payload.Preferences, &next.Preferences); err != nil {
			return errors.New("备份中的偏好设置无效")
		}
		if report.Format != BackupNative && !slices.Equal(s.Preferences.BlockedKeywords, next.Preferences.BlockedKeywords) {
			next.Preferences.BlockedKeywords = mergeKeywords(s.Preferences.BlockedKeywords, next.Preferences.BlockedKeywords)
		}
	}
	next.Preferences.Volume = max(0, min(100, next.Preferences.Volume))
	next.Preferences.DefaultQuality = platforms.NormalizeQualityPreference(next.Preferences.DefaultQuality)
	known := make(map[string]bool, len(next.Favorites)+len(payload.Favorites))
	for _, room := range next.Favorites {
		known[FavoriteKey(room)] = true
	}
	for _, room := range payload.Favorites {
		if room.ID == "" || strings.ContainsAny(room.ID, "\r\n\x00") {
			return errors.New("备份包含无效的直播间")
		}
		switch room.Platform {
		case platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin, platforms.Qie, platforms.FLive:
		default:
			return errors.New("备份包含无法识别的来源")
		}
		key := FavoriteKey(room)
		if !known[key] {
			next.Favorites = append(next.Favorites, room)
			known[key] = true
			report.Added++
		} else {
			report.Existing++
		}
		if details, ok := payload.Details[key]; ok {
			var err error
			details, err = normalizeFavoriteDetails(details)
			if err != nil {
				return err
			}
			if current, exists := next.FavoriteDetails[key]; exists && report.Format != BackupNative {
				if current.Group == "" {
					current.Group = details.Group
				}
				current.Pinned = current.Pinned || details.Pinned
				details = current
			}
			next.FavoriteDetails[key] = details
		}
	}
	*s = next
	if err := s.save(); err != nil {
		*s = previous
		return err
	}
	return nil
}
