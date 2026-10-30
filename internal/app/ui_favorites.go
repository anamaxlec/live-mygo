//go:build darwin

package app

import (
	"context"
	"live-mygo/internal/platforms"
	"sort"
	"sync"
	"time"
)

func roomKey(r platforms.Room) string { return string(r.Platform) + ":" + r.ID }
func (a *app) favoriteRooms() []platforms.Room {
	if a.library == nil {
		return nil
	}
	rooms := append([]platforms.Room(nil), a.library.Favorites...)
	for i, r := range rooms {
		if fresh, ok := a.libraryRooms[roomKey(r)]; ok {
			rooms[i] = fresh
		}
	}
	sort.SliceStable(rooms, func(i, j int) bool {
		x, xok := a.libraryRooms[roomKey(rooms[i])]
		y, yok := a.libraryRooms[roomKey(rooms[j])]
		return xok && x.Live && (!yok || !y.Live)
	})
	return rooms
}

func (a *app) refreshFavorites(force bool) {
	if a.library == nil || a.win == nil || a.libraryRefreshing || len(a.library.Favorites) == 0 {
		return
	}
	if !force && time.Since(a.libraryRefresh) < time.Minute {
		return
	}
	a.libraryRefreshing = true
	rooms := append([]platforms.Room(nil), a.library.Favorites...)
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		jobs := make(chan platforms.Room, len(rooms))
		for _, r := range rooms {
			jobs <- r
		}
		close(jobs)
		results := map[string]platforms.Room{}
		var mu sync.Mutex
		var wg sync.WaitGroup
		for range min(3, len(rooms)) {
			wg.Go(func() {
				for old := range jobs {
					if ctx.Err() != nil {
						return
					}
					req, done := context.WithTimeout(ctx, 8*time.Second)
					r, err := a.client.RoomInfo(req, old.Platform, old.ID)
					done()
					if err != nil {
						continue
					}
					if r.Anchor == "" {
						r.Anchor = old.Anchor
					}
					if r.Cover == "" {
						r.Cover = old.Cover
					}
					// 收藏的身份不因平台别名解析而变化。
					r.ID = old.ID
					mu.Lock()
					results[roomKey(old)] = r
					mu.Unlock()
				}
			})
		}
		wg.Wait()
		a.win.Update(func() {
			if a.closing {
				return
			}
			a.libraryRefreshing = false
			a.libraryRefresh = time.Now()
			a.libraryRooms = results
			if len(results) < len(rooms) {
				a.toast = "部分直播间状态暂未更新，可稍后刷新"
			}
		})
	}()
}
