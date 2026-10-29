package library

import (
	"live-mygo/internal/platforms"
	"strings"
)

func (p Preferences) Blocks(platform platforms.Platform, msg platforms.ChatMessage) bool {
	if !p.FilterEnabled {
		return false
	}
	text := strings.ToLower(msg.Text)
	for _, keyword := range p.BlockedKeywords {
		if keyword = strings.TrimSpace(keyword); keyword != "" && strings.Contains(text, strings.ToLower(keyword)) {
			return true
		}
	}
	for _, user := range p.BlockedUsers {
		if user.Platform != platform {
			continue
		}
		if user.ID != "" && msg.UserID != "" {
			if user.ID == msg.UserID {
				return true
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(user.Name), strings.TrimSpace(msg.User)) {
			return true
		}
	}
	return false
}
