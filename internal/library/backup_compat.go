package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"live-mygo/internal/platforms"
)

type BackupFormat string

const (
	BackupNative         BackupFormat = "Live MyGo"
	BackupSimple         BackupFormat = "Simple Live"
	BackupSimpleSettings BackupFormat = "Simple Live 旧版设置"
	BackupSimpleFollows  BackupFormat = "Simple Live 旧版关注"
	BackupPure           BackupFormat = "纯粹直播"
	BackupPureFollows    BackupFormat = "纯粹直播仅关注"
)

func (f BackupFormat) Filename() string {
	switch f {
	case BackupSimple:
		return "simple-live-profile.json"
	case BackupSimpleSettings:
		return "simple_live_config.json"
	case BackupSimpleFollows:
		return "simple-live-follows.json"
	case BackupPure:
		return "pure-live-backup.txt"
	case BackupPureFollows:
		return "pure-live-favorites.txt"
	default:
		return "LiveMyGo-backup-" + time.Now().Format("20060102") + ".json"
	}
}

type backupObject = map[string]any

func object(v any) backupObject { m, _ := v.(map[string]any); return m }
func items(v any) []any         { a, _ := v.([]any); return a }
func field(v any) string        { s, _ := v.(string); return s }
func identifier(v any) string {
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	return strings.TrimSpace(field(v))
}

func compatibleSource(p platforms.Platform) bool {
	switch p {
	case platforms.Bilibili, platforms.Douyu, platforms.Huya, platforms.Douyin:
		return true
	default:
		return false
	}
}

func decodeBackup(data []byte) (backupPayload, ImportReport, error) {
	var decoded any
	var payload backupPayload
	var report ImportReport
	data = bytes.TrimPrefix(bytes.TrimSpace(data), []byte{0xef, 0xbb, 0xbf})
	if !decodeBackupJSON(data, &decoded) {
		return payload, report, errors.New("无法读取这个 JSON 备份")
	}
	root := object(decoded)
	if field(root["type"]) == "pure_live_sync" {
		root = object(root["settings"])
	}
	var prefs backupObject
	switch {
	case field(root["schema"]) == "simple_live_profile":
		report.Format = BackupSimple
		if n := identifier(root["schemaVersion"]); n != "" && !slices.Contains([]string{"1", "2", "3", "4"}, n) {
			return payload, report, errors.New("不支持这个 Simple Live 配置包版本")
		}
		prefs = simplePreferences(object(root["settings"]))
		shield := object(root["danmuShield"])
		keywords := stringItems(shield["keywords"])
		if len(keywords) == 0 {
			for _, raw := range stringItems(shield["raw"]) {
				if word, ok := strings.CutPrefix(raw, "keyword:"); ok {
					keywords = append(keywords, word)
				} else if !strings.Contains(raw, ":") {
					keywords = append(keywords, raw)
				}
			}
		}
		setKeywords(prefs, keywords)
		payload.Favorites, payload.Details = foreignRooms(simpleRows(root), simpleGroups(root), false, &report)
	case field(root["type"]) == "simple_live":
		report.Format = BackupSimpleSettings
		prefs = simplePreferences(object(root["config"]))
		words := stringItems(root["shield"])
		for _, v := range object(root["shield"]) {
			if word := field(v); word != "" {
				words = append(words, word)
			}
		}
		slices.Sort(words)
		setKeywords(prefs, words)
	case items(decoded) != nil || simpleRows(root) != nil:
		report.Format = BackupSimpleFollows
		rows := items(decoded)
		if rows == nil {
			rows = simpleRows(root)
		}
		payload.Favorites, payload.Details = foreignRooms(rows, simpleGroups(root), false, &report)
	case root["backupVersion"] != nil || root["favorite"] != nil || root["favoriteRooms"] != nil || root["hideDanmaku"] != nil || root["themeMode"] != nil:
		report.Format = BackupPure
		favorite := object(root["favorite"])
		if favorite == nil {
			favorite = root
		}
		prefs = purePreferences(root)
		setKeywords(prefs, stringItems(favorite["shieldList"]))
		rows := pureRows(favorite["favoriteRooms"])
		payload.Favorites, payload.Details = foreignRooms(rows, pureGroups(root, rows), true, &report)
	default:
		report.Format = BackupNative
		if err := json.Unmarshal(data, &payload); err != nil {
			return payload, report, errors.New("无法读取这个备份文件")
		}
		report.Settings = len(object(root["preferences"]))
		return payload, report, nil
	}
	if len(prefs) > 0 {
		report.Settings = len(prefs)
		var err error
		payload.Preferences, err = json.Marshal(prefs)
		if err != nil {
			return payload, report, err
		}
	}
	if len(payload.Favorites) == 0 && len(payload.Preferences) == 0 && report.Skipped > 0 {
		return payload, report, errors.New("备份中的关注来源不受支持，或房间号无效")
	}
	return payload, report, nil
}

func decodeBackupJSON(data []byte, dst *any) bool {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(dst) == nil && dec.Decode(new(any)) == io.EOF
}

func wrappedItems(value any) []any {
	if rows := items(value); rows != nil {
		return rows
	}
	return items(object(value)["data"])
}

func simpleRows(root backupObject) []any {
	for _, key := range []string{"data", "followUsers", "follows"} {
		if rows := wrappedItems(root[key]); rows != nil {
			return rows
		}
	}
	// "favorites" is also the native field name. Foreign rows have a siteId.
	rows := wrappedItems(root["favorites"])
	if len(rows) > 0 && object(rows[0])["siteId"] != nil {
		return rows
	}
	return nil
}

func pureRows(value any) []any {
	if raw, ok := value.(string); ok && !decodeBackupJSON([]byte(raw), &value) {
		return nil
	}
	rows := items(value)
	if rows == nil {
		rows = items(object(value)["list"])
	}
	for i, v := range rows {
		if raw, ok := v.(string); ok && decodeBackupJSON([]byte(raw), &v) {
			rows[i] = v
		}
	}
	return rows
}

func foreignRooms(rows []any, groups map[string]string, pure bool, report *ImportReport) ([]platforms.Room, map[string]FavoriteDetails) {
	rooms := make([]platforms.Room, 0, len(rows))
	details := make(map[string]FavoriteDetails)
	for _, v := range rows {
		row := object(v)
		if row == nil || !pure && row["updateTime"] != nil {
			report.Skipped++
			continue
		}
		platform, name, title := "siteId", "userName", "roomTitle"
		if pure {
			platform, name, title = "platform", "nick", "title"
		}
		src := platforms.Platform(strings.ToLower(identifier(row[platform])))
		id := identifier(row["roomId"])
		if !compatibleSource(src) || strings.Contains(id, "://") {
			report.Skipped++
			continue
		}
		if _, _, err := platforms.ParseRoom(src, id); err != nil {
			report.Skipped++
			continue
		}
		room := platforms.Room{Platform: src, ID: id, Anchor: field(row[name]), Title: field(row[title])}
		rooms = append(rooms, room)
		group := field(row["tag"])
		if group == "" || group == "全部" {
			group = groups[identifier(row["id"])]
			if group == "" {
				group = groups[string(src)+"_"+id]
			}
		}
		if pure {
			group = groups[FavoriteKey(room)]
		}
		groupRunes := []rune(strings.TrimSpace(group))
		group = string(groupRunes[:min(24, len(groupRunes))])
		pinned, _ := row["isSpecialFollow"].(bool)
		pinned = pinned || identifier(row["isSpecialFollow"]) == "1"
		if group != "" || pinned {
			details[FavoriteKey(room)] = FavoriteDetails{Group: group, Pinned: pinned}
		}
	}
	return rooms, details
}

func simpleGroups(root backupObject) map[string]string {
	groups := map[string]string{}
	tags := wrappedItems(root["followUserTags"])
	if tags == nil {
		tags = wrappedItems(root["tags"])
	}
	for _, v := range tags {
		tag := object(v)
		for _, id := range stringItems(tag["userId"]) {
			if groups[id] == "" {
				groups[id] = field(tag["tag"])
			}
		}
	}
	return groups
}

func pureGroups(root backupObject, rows []any) map[string]string {
	tags := object(root["tags"])
	if tags == nil {
		tags = object(root["custom_tags_data"])
	}
	names := map[string]string{}
	for _, v := range items(tags["tags"]) {
		tag := object(v)
		names[identifier(tag["id"])] = field(tag["name"])
	}
	groups := map[string]string{}
	for _, v := range rows {
		r := object(v)
		id := identifier(r["roomId"])
		key := strings.ToLower(identifier(r["platform"])) + ":" + id
		for _, ids := range []any{r["tagIds"], object(tags["roomTagsMap"])[key], object(tags["roomTagsMap"])[id]} {
			for _, tag := range stringItems(ids) {
				if groups[key] == "" {
					groups[key] = names[tag]
				}
			}
		}
	}
	return groups
}

func stringItems(value any) []string {
	var result []string
	for _, v := range items(value) {
		if s := field(v); s != "" {
			result = append(result, s)
		}
	}
	return result
}

func mergeKeywords(a, b []string) []string {
	var result []string
	seen := map[string]bool{}
	for _, values := range [][]string{a, b} {
		for _, value := range values {
			value = strings.TrimSpace(value)
			key := strings.ToLower(value)
			if value != "" && !seen[key] {
				result = append(result, value)
				seen[key] = true
			}
		}
	}
	return result
}

func setKeywords(prefs backupObject, words []string) {
	if len(words) > 0 {
		prefs["blocked_keywords"] = mergeKeywords(nil, words)
	}
}

func numberSetting(dst backupObject, key string, src backupObject, field string, scale, lo, hi float64) {
	if n, ok := src[field].(json.Number); ok {
		if v, err := n.Float64(); err == nil {
			dst[key] = max(lo, min(hi, v*scale))
		}
	}
}

func boolSetting(dst backupObject, key string, src backupObject, field string, inverted bool) {
	if b, ok := src[field].(bool); ok {
		dst[key] = b != inverted
	}
}

func simplePreferences(src backupObject) backupObject {
	dst := backupObject{}
	if i, err := strconv.Atoi(identifier(src["ThemeMode"])); err == nil && i >= 0 && i <= 2 {
		dst["appearance"] = []string{"system", "light", "dark"}[i]
	}
	boolSetting(dst, "danmaku", src, "DanmuEnable", false)
	boolSetting(dst, "filter_enabled", src, "DanmuShieldEnable", false)
	boolSetting(dst, "filter_duplicates", src, "DanmuDedupeEnable", false)
	numberSetting(dst, "volume", src, "PlayerVolume", 1, 0, 100)
	numberSetting(dst, "danmaku_size", src, "DanmuSize", 1, 16, 48)
	numberSetting(dst, "danmaku_opacity", src, "DanmuOpacity", 100, 20, 100)
	numberSetting(dst, "danmaku_area", src, "DanmuArea", 1, .25, 1)
	numberSetting(dst, "danmaku_stroke", src, "DanmuStrokeWidth", 1, 0, 4)
	return dst
}

func pureSection(root backupObject, key string) backupObject {
	if m := object(root[key]); m != nil {
		return m
	}
	return root
}

func purePreferences(root backupObject) backupObject {
	dst := backupObject{}
	if theme := strings.ToLower(field(pureSection(root, "theme")["themeMode"])); slices.Contains([]string{"system", "light", "dark"}, theme) {
		dst["appearance"] = theme
	}
	src := pureSection(root, "danmaku")
	boolSetting(dst, "danmaku", src, "hideDanmaku", true)
	if enabled, ok := src["enableDanmakuDisplay"].(bool); ok && (!enabled || dst["danmaku"] == nil) {
		dst["danmaku"] = enabled
	}
	boolSetting(dst, "filter_duplicates", src, "collapseRepeatedDanmaku", false)
	numberSetting(dst, "volume", pureSection(root, "volume"), "defaultDesktopVolume", 100, 0, 100)
	if _, ok := dst["volume"]; !ok {
		numberSetting(dst, "volume", pureSection(root, "volume"), "defaultMobileVolume", 100, 0, 100)
	}
	numberSetting(dst, "danmaku_size", src, "danmakuFontSize", 1, 16, 48)
	numberSetting(dst, "danmaku_opacity", src, "danmakuOpacity", 100, 20, 100)
	numberSetting(dst, "danmaku_area", src, "danmakuArea", 1, .25, 1)
	numberSetting(dst, "danmaku_stroke", src, "danmakuFontBorder", 1, 0, 4)
	if enabled, ok := src["enableDanmakuStroke"].(bool); ok && !enabled {
		dst["danmaku_stroke"] = 0.0
	}
	return dst
}
