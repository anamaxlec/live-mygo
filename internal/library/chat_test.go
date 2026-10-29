package library

import (
	"live-mygo/internal/platforms"
	"testing"
)

func TestChatBlockingUsesPlatformAndStableUserIdentity(t *testing.T) {
	prefs := Preferences{FilterEnabled: true, BlockedKeywords: []string{"广告", "spam"}, BlockedUsers: []BlockedUser{{Platform: platforms.Bilibili, ID: "42", Name: "旧昵称"}}}
	cases := []struct {
		platform platforms.Platform
		msg      platforms.ChatMessage
		blocked  bool
	}{
		{platforms.Bilibili, platforms.ChatMessage{UserID: "42", User: "新昵称", Text: "正常发言"}, true},
		{platforms.Douyu, platforms.ChatMessage{UserID: "42", User: "新昵称", Text: "正常发言"}, false},
		{platforms.Bilibili, platforms.ChatMessage{UserID: "43", User: "旧昵称", Text: "正常发言"}, false},
		{platforms.Douyu, platforms.ChatMessage{Text: "SPAM 测试"}, true},
		{platforms.Bilibili, platforms.ChatMessage{Text: "广告测试"}, true},
	}
	for _, tc := range cases {
		if got := prefs.Blocks(tc.platform, tc.msg); got != tc.blocked {
			t.Fatalf("%+v blocked=%v", tc, got)
		}
	}
	prefs.FilterEnabled = false
	if prefs.Blocks(platforms.Bilibili, cases[0].msg) {
		t.Fatal("禁用规则后仍屏蔽消息")
	}
}
