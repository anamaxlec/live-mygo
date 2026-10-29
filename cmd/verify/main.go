package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"live-mygo/internal/platforms"
)

func main() {
	p := flag.String("platform", "bilibili", "bilibili 或 douyu")
	r := flag.String("room", "", "房间号或官方直播间链接")
	q := flag.Int("quality", -1, "清晰度编号，-1 为默认")
	route := flag.String("source", "", "平台实际返回的线路 ID")
	catalog := flag.Bool("catalog", false, "验证直播列表")
	query := flag.String("query", "", "验证主播或直播间搜索")
	categories := flag.Bool("categories", false, "验证平台分区")
	category := flag.String("category", "", "验证分区列表")
	parent := flag.String("parent", "", "分区所属的一级分类")
	chat := flag.Bool("chat", false, "验证弹幕连接，仅输出连接状态与条数")
	chatDuration := flag.Duration("chat-duration", 15*time.Second, "弹幕验证持续时间")
	douyuTLS := flag.Bool("douyu-compatible-tls", false, "仅斗鱼公开弹幕启用 TLS 1.2 RSA AES-GCM 兼容连接")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	start := time.Now()
	if *chat {
		chatCtx, chatCancel := context.WithTimeout(ctx, *chatDuration)
		defer chatCancel()
		var status string
		var count int
		client := platforms.NewClient(nil)
		client.SetDouyuChatCompatibility(*douyuTLS)
		err := client.ListenChat(chatCtx, platforms.Platform(*p), *r, func(platforms.ChatMessage) { count++ }, func(value string) { status = value })
		var connection *platforms.ChatConnectionError
		if errors.As(err, &connection) {
			fmt.Fprintln(os.Stderr, connection.Cause)
		}
		if err != nil && chatCtx.Err() == nil {
			status = err.Error()
		}
		json.NewEncoder(os.Stdout).Encode(struct {
			Status   string `json:"status"`
			Messages int    `json:"messages"`
		}{status, count})
		return
	}
	if *categories {
		result, err := platforms.NewClient(nil).Categories(ctx, platforms.Platform(*p))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}
	if *catalog || *query != "" || *category != "" {
		client := platforms.NewClient(nil)
		var result platforms.Catalog
		var err error
		if *query != "" {
			result, err = client.Search(ctx, platforms.Platform(*p), *query, 1)
		} else if *category != "" {
			result, err = client.CategoryRooms(ctx, platforms.Platform(*p), platforms.Category{ID: *category, ParentID: *parent}, 1)
		} else {
			result, err = client.Recommend(ctx, platforms.Platform(*p), 1)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(result)
		return
	}
	s, err := platforms.NewClient(nil).Resolve(ctx, platforms.Platform(*p), *r, *q, *route)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	result := struct {
		*platforms.Stream
		ResolveMS int64 `json:"resolve_ms"`
	}{s, time.Since(start).Milliseconds()}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
