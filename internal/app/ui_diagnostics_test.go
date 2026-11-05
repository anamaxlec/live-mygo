//go:build darwin || windows

package app

import (
	"encoding/json"
	"strings"
	"testing"

	"live-mygo/internal/platforms"
)

func TestDiagnosticsContainsPlaybackFactsWithoutSensitiveInputs(t *testing.T) {
	a := polishApp(t)
	secret := "secret-cookie-and-signed-token"
	a.stream = &platforms.Stream{URL: "https://cdn.invalid/live?token=" + secret, Headers: map[string]string{"Cookie": secret}}
	a.status = "failed " + secret
	a.stats.Error = "https://cdn.invalid/" + secret
	a.stats.Width, a.stats.Height, a.stats.Hardware = 1920, 1080, "videotoolbox"
	a.recordFailure("video", "timeout https://cdn.invalid/?token="+secret)
	data, err := a.diagnosticData()
	if err != nil {
		t.Fatal(err)
	}
	for _, sensitive := range []string{secret, "cdn.invalid", "Cookie", "Headers", "token="} {
		if strings.Contains(string(data), sensitive) {
			t.Fatal("诊断报告泄露敏感输入", sensitive)
		}
	}
	var decoded map[string]any
	if json.Unmarshal(data, &decoded) != nil || !strings.Contains(string(data), "videotoolbox") || !strings.Contains(string(data), "timeout") {
		t.Fatal("诊断报告丢失播放或错误分类")
	}
}
