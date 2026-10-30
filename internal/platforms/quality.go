package platforms

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	QualityHighest  = "highest"
	QualityOriginal = "original"
	QualityBluRay   = "bluray"
	QualityUltra    = "ultra"
	QualityHigh     = "high"
	QualitySmooth   = "smooth"
)

// NormalizeQualityPreference 将缺失或旧版本的默认画质统一为最高画质。
func NormalizeQualityPreference(value string) string {
	switch value {
	case QualityOriginal, QualityBluRay, QualityUltra, QualityHigh, QualitySmooth:
		return value
	default:
		return QualityHighest
	}
}

var (
	qualityResolution = regexp.MustCompile(`(?i)(\d{3,4})p`)
	qualityBitrate    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*m(?:bps)?`)
)

type qualityRank struct {
	tier     int
	bitrate  float64
	height   int
	ignored  bool
	sourceAt int
}

func rankQuality(q Quality, index int) qualityRank {
	name := strings.ToLower(strings.ReplaceAll(q.Name, " ", ""))
	rank := qualityRank{sourceAt: index}
	switch {
	case strings.Contains(name, "杜比") || strings.Contains(name, "4k"):
		rank.tier = 8
	case strings.Contains(name, "2k"):
		rank.tier = 7
	case strings.Contains(name, "原画"):
		rank.tier = 6
	case strings.Contains(name, "蓝光"):
		rank.tier = 5
	case strings.Contains(name, "超清"):
		rank.tier = 4
	case strings.Contains(name, "高清"):
		rank.tier = 3
	case strings.Contains(name, "标清"):
		rank.tier = 2
	case strings.Contains(name, "流畅"):
		rank.tier = 1
	}
	if match := qualityResolution.FindStringSubmatch(name); len(match) > 1 {
		height, _ := strconv.Atoi(match[1])
		rank.height = height
		if rank.tier == 0 {
			switch {
			case height >= 2160:
				rank.tier = 8
			case height >= 1440:
				rank.tier = 7
			case height >= 1080:
				rank.tier = 4
			case height >= 720:
				rank.tier = 3
			case height >= 480:
				rank.tier = 2
			default:
				rank.tier = 1
			}
		}
	}
	if match := qualityBitrate.FindStringSubmatch(name); len(match) > 1 {
		rank.bitrate, _ = strconv.ParseFloat(match[1], 64)
	}
	// “自动”不是具体档位，设置默认画质时跳过。
	if strings.TrimSpace(q.Name) == "自动" {
		rank.ignored = true
	}
	return rank
}

func betterQuality(a, b qualityRank) bool {
	if a.tier != b.tier {
		return a.tier > b.tier
	}
	if a.bitrate != b.bitrate {
		return a.bitrate > b.bitrate
	}
	if a.height != b.height {
		return a.height > b.height
	}
	return a.sourceAt < b.sourceAt
}

// PreferredQuality 根据默认画质策略从平台返回的列表中选择档位。
// 平台没有指定档位时选择不高于目标档位的最高可用档位。
func PreferredQuality(qualities []Quality, preference string) (int, bool) {
	preference = NormalizeQualityPreference(preference)
	if len(qualities) == 0 {
		return 0, false
	}
	ranks := make([]qualityRank, len(qualities))
	ignored := true
	for i, q := range qualities {
		ranks[i] = rankQuality(q, i)
		if !ranks[i].ignored {
			ignored = false
		}
	}
	if ignored {
		return qualities[0].ID, true
	}
	target := 0
	switch preference {
	case QualityOriginal:
		target = 6
	case QualityBluRay:
		target = 5
	case QualityUltra:
		target = 4
	case QualityHigh:
		target = 3
	case QualitySmooth:
		target = 1
	default:
		target = 9
	}
	best, found := -1, false
	for i, rank := range ranks {
		if rank.ignored {
			continue
		}
		if target < 9 && rank.tier > target {
			continue
		}
		if !found || betterQuality(rank, ranks[best]) {
			best, found = i, true
		}
	}
	if !found {
		// 目标档位低于所有可用档位时退回最低档。
		for i, rank := range ranks {
			if rank.ignored {
				continue
			}
			if !found || rank.tier < ranks[best].tier || rank.tier == ranks[best].tier && (rank.bitrate < ranks[best].bitrate || rank.bitrate == ranks[best].bitrate && rank.height < ranks[best].height) {
				best, found = i, true
			}
		}
	}
	if !found {
		return qualities[0].ID, true
	}
	return qualities[best].ID, true
}
