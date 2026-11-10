package library

import (
	"encoding/json"
	"errors"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (s *State) BackupAs(format BackupFormat) ([]byte, int, error) {
	if format == BackupNative || format == "" {
		data, err := s.Backup()
		return data, 0, err
	}
	var payload any
	var skipped int
	switch format {
	case BackupSimple, BackupSimpleFollows, BackupSimpleSettings:
		payload, skipped = s.simpleBackup(format)
	case BackupPure, BackupPureFollows:
		payload, skipped = s.pureBackup(format)
	default:
		return nil, 0, errors.New("不支持的导出格式")
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	return data, skipped, err
}

func (s *State) simpleBackup(format BackupFormat) (any, int) {
	p := s.Preferences
	theme := max(0, slices.Index([]string{"system", "light", "dark"}, p.Appearance))
	settings := backupObject{
		"ThemeMode": theme, "PlayerVolume": simpleDouble(p.Volume),
		"DanmuEnable": p.Danmaku, "DanmuSize": simpleDouble(p.DanmakuSize),
		"DanmuOpacity": simpleDouble(p.DanmakuOpacity / 100), "DanmuArea": simpleDouble(p.DanmakuArea),
		"DanmuStrokeWidth": simpleDouble(p.DanmakuStroke), "DanmuShieldEnable": p.FilterEnabled,
		"DanmuDedupeEnable": p.FilterDuplicates,
	}
	keywords := append([]string{}, p.BlockedKeywords...)
	now := time.Now()
	platform := runtime.GOOS
	if platform == "darwin" {
		platform = "macos"
	}
	if format == BackupSimpleSettings {
		shield := make(map[string]string, len(keywords))
		for _, word := range keywords {
			shield[word] = word
		}
		return backupObject{"type": "simple_live", "platform": platform, "version": 1, "time": now.UnixMilli(), "config": settings, "shield": shield}, 0
	}
	rows := []backupObject{}
	tags := make(map[string][]string)
	skipped := 0
	for _, room := range s.Favorites {
		if !compatibleSource(room.Platform) {
			skipped++
			continue
		}
		d := s.Details(room)
		id := string(room.Platform) + "_" + room.ID
		tag := d.Group
		if tag == "" {
			tag = "全部"
		} else {
			tags[tag] = append(tags[tag], id)
		}
		row := backupObject{"id": id, "roomId": room.ID, "siteId": room.Platform,
			"userName": room.Anchor, "face": "", "addTime": now.Format(time.RFC3339),
			"tag": tag, "isSpecialFollow": d.Pinned}
		if format != BackupSimpleFollows {
			row["roomTitle"] = room.Title
		}
		rows = append(rows, row)
	}
	if format == BackupSimpleFollows {
		return rows, skipped
	}
	followTags := []backupObject{}
	for _, tag := range s.FavoriteGroups() {
		if len(tags[tag]) > 0 {
			followTags = append(followTags, backupObject{"id": "livemygo:" + tag, "tag": tag, "userId": tags[tag]})
		}
	}
	return backupObject{
		"schema": "simple_live_profile", "schemaVersion": 4, "platform": platform, "exportedAt": now.Format(time.RFC3339),
		"included": backupObject{"settings": true, "shields": true, "follows": true},
		"settings": settings, "followUsers": rows, "followUserTags": followTags,
		"danmuShield": backupObject{"keywords": keywords},
	}, skipped
}

// Simple Live's settings store casts these JSON values to Dart double.
func simpleDouble(value float64) json.Number {
	number := strconv.FormatFloat(value, 'f', -1, 64)
	if !strings.Contains(number, ".") {
		number += ".0"
	}
	return json.Number(number)
}

func (s *State) pureBackup(format BackupFormat) (any, int) {
	p := s.Preferences
	rooms := []backupObject{}
	tags := []backupObject{}
	assignments := map[string][]string{}
	tagIDs := make(map[string]string)
	skipped := 0
	for _, room := range s.Favorites {
		if !compatibleSource(room.Platform) {
			skipped++
			continue
		}
		row := backupObject{"roomId": room.ID, "platform": room.Platform, "nick": room.Anchor, "title": room.Title}
		if group := s.Details(room).Group; group != "" && format != BackupPureFollows {
			id := tagIDs[group]
			if id == "" {
				id = "livemygo:" + group
				tagIDs[group] = id
				tags = append(tags, backupObject{"id": id, "name": group, "description": "", "order": len(tags)})
			}
			assignments[FavoriteKey(room)] = []string{id}
			row["tagIds"] = []string{id}
		}
		rooms = append(rooms, row)
	}
	favorite := backupObject{"favoriteRooms": rooms}
	result := backupObject{"backupVersion": 3, "sensitiveDataIncluded": false, "favorite": favorite}
	if format == BackupPureFollows {
		result["backupScope"] = "favorites"
		return result, skipped
	}
	favorite["shieldList"] = append([]string{}, p.BlockedKeywords...)
	theme := "System"
	if p.Appearance == "light" || p.Appearance == "dark" {
		theme = strings.ToUpper(p.Appearance[:1]) + p.Appearance[1:]
	}
	result["theme"] = backupObject{"themeMode": theme}
	result["volume"] = backupObject{"defaultDesktopVolume": p.Volume / 100, "defaultMobileVolume": p.Volume / 100}
	result["danmaku"] = backupObject{
		"hideDanmaku": !p.Danmaku, "danmakuFontSize": max(10, min(30, p.DanmakuSize)),
		"danmakuOpacity": p.DanmakuOpacity / 100, "danmakuArea": p.DanmakuArea,
		"danmakuFontBorder": p.DanmakuStroke, "enableDanmakuStroke": p.DanmakuStroke > 0,
		"collapseRepeatedDanmaku": p.FilterDuplicates,
	}
	if len(tags) > 0 {
		result["tags"] = backupObject{"tags": tags, "roomTagsMap": assignments}
	}
	return result, skipped
}
