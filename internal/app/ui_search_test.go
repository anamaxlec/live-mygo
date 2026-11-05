//go:build darwin || windows

package app

import (
	"context"
	"testing"
	"time"

	"live-mygo/internal/platforms"
)

func TestLocalSearchPrefersFavoritesAndKeepsSourceIdentity(t *testing.T) {
	a := polishApp(t)
	favorite := platforms.Room{Platform: platforms.Douyu, ID: "1", Anchor: "测试主播"}
	other := platforms.Room{Platform: platforms.Bilibili, ID: "1", Anchor: "测试主播"}
	a.library.Favorites = []platforms.Room{favorite}
	a.library.History = []platforms.Room{other, favorite}
	rooms := a.localSearchRooms("测试", "")
	if len(rooms) != 2 || rooms[0].Platform != platforms.Douyu {
		t.Fatal("本地结果丢失来源身份或关注优先级", rooms)
	}
	if filtered := a.localSearchRooms("测试", platforms.Bilibili); len(filtered) != 1 || filtered[0].Platform != platforms.Bilibili {
		t.Fatal("来源筛选失效", filtered)
	}
}

func TestUnifiedSearchReturnsFastSourceWithoutWaitingForSlowSource(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	results := searchAcrossSources(ctx, []platforms.Platform{platforms.Bilibili, platforms.Douyu}, "主播", func(ctx context.Context, p platforms.Platform, query string) ([]platforms.Room, error) {
		if p == platforms.Bilibili {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []platforms.Room{{Platform: p, ID: "1"}}, nil
	})
	select {
	case result := <-results:
		if len(result.rooms) != 1 || result.rooms[0].Platform != platforms.Douyu {
			t.Fatal("较慢来源阻塞了结果", result)
		}
	case <-time.After(time.Second):
		t.Fatal("没有及时返回较快来源")
	}
	cancel()
	for range results {
	}
}
