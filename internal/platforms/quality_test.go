package platforms

import "testing"

func TestPreferredQualityAcrossPlatforms(t *testing.T) {
	douyu := []Quality{{0, "原画1080P30"}, {4, "蓝光4M"}, {3, "超清"}, {2, "高清"}}
	bilibili := []Quality{{30000, "杜比"}, {20000, "4K"}, {10000, "原画"}, {400, "蓝光"}, {250, "超清"}, {150, "高清"}, {80, "流畅"}}
	huya := []Quality{{0, "蓝光4M"}, {2000, "超清"}, {1000, "高清"}}
	douyin := []Quality{{4, "原画"}, {3, "蓝光"}, {2, "超清"}, {1, "高清"}}
	qie := []Quality{{0, "原画"}, {1, "超清"}, {2, "高清"}, {3, "标清"}}
	flive := []Quality{{0, "自动"}, {1, "1080p"}, {2, "720p"}, {3, "480p"}}
	tests := []struct {
		name       string
		qualities  []Quality
		preference string
		want       int
	}{
		{"斗鱼最高", douyu, QualityHighest, 0},
		{"斗鱼原画", douyu, QualityOriginal, 0},
		{"斗鱼蓝光", douyu, QualityBluRay, 4},
		{"斗鱼超清", douyu, QualityUltra, 3},
		{"斗鱼高清", douyu, QualityHigh, 2},
		{"斗鱼流畅回退最低", douyu, QualitySmooth, 2},
		{"B站最高", bilibili, QualityHighest, 30000},
		{"B站蓝光", bilibili, QualityBluRay, 400},
		{"B站超清", bilibili, QualityUltra, 250},
		{"B站高清", bilibili, QualityHigh, 150},
		{"B站流畅", bilibili, QualitySmooth, 80},
		{"虎牙最高", huya, QualityHighest, 0},
		{"虎牙超清", huya, QualityUltra, 2000},
		{"虎牙流畅回退最低", huya, QualitySmooth, 1000},
		{"抖音最高", douyin, QualityHighest, 4},
		{"抖音蓝光", douyin, QualityBluRay, 3},
		{"抖音流畅", douyin, QualitySmooth, 1},
		{"企鹅最高", qie, QualityHighest, 0},
		{"企鹅蓝光回退超清", qie, QualityBluRay, 1},
		{"企鹅标清回退", qie, QualitySmooth, 3},
		{"赛事最高跳过自动", flive, QualityHighest, 1},
		{"赛事高清", flive, QualityHigh, 2},
		{"赛事流畅", flive, QualitySmooth, 3},
		{"无低档位时回退唯一档", []Quality{{1, "1080p"}}, QualitySmooth, 1},
		{"只有自动时保留自动", []Quality{{0, "自动"}}, QualityHighest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PreferredQuality(tt.qualities, tt.preference)
			if !ok || got != tt.want {
				t.Fatalf("PreferredQuality(%v, %q) = %d, %v; want %d, true", tt.qualities, tt.preference, got, ok, tt.want)
			}
		})
	}
}

func TestPreferredQualityEmpty(t *testing.T) {
	if id, ok := PreferredQuality(nil, QualityHighest); ok {
		t.Fatalf("空清晰度列表不应返回档位：%d", id)
	}
}

func TestNormalizeQualityPreference(t *testing.T) {
	for _, value := range []string{QualityHighest, QualityOriginal, QualityBluRay, QualityUltra, QualityHigh, QualitySmooth} {
		if got := NormalizeQualityPreference(value); got != value {
			t.Fatalf("NormalizeQualityPreference(%q) = %q", value, got)
		}
	}
	for _, value := range []string{"", "bogus", "自动"} {
		if got := NormalizeQualityPreference(value); got != QualityHighest {
			t.Fatalf("NormalizeQualityPreference(%q) = %q; want %q", value, got, QualityHighest)
		}
	}
}
