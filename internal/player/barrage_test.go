//go:build darwin

package player

import (
	"strings"
	"testing"
)

func TestNativeOverlayNamedArguments(t *testing.T) {
	p, err := New(nil)
	if err != nil {
		if strings.Contains(err.Error(), "找不到 libmpv") {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer p.Close()
	if err = p.namedCommand("osd-overlay", map[string]any{"id": 7, "format": "ass-events", "data": `{\pos(20,20)}原生叠加验证`, "res_x": 1280, "res_y": 720}); err != nil {
		t.Fatal(err)
	}
	if err = p.namedCommand("osd-overlay", map[string]any{"id": 7, "format": "none", "data": ""}); err != nil {
		t.Fatal(err)
	}
}

func TestChatCannotInjectASSCommands(t *testing.T) {
	escaped := escapeASS("hello{\\pos(0,0)}\\Nnext\nline")
	if strings.Contains(escaped, `\pos`) || strings.Contains(escaped, `\N`) || strings.ContainsRune(escaped, '\n') {
		t.Fatalf("未转义外部弹幕：%q", escaped)
	}
}
