package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// RoomInfo 只读取房间元信息与开播状态，不请求取流签名或启动播放器。
func (c *Client) RoomInfo(ctx context.Context, p Platform, id string) (Room, error) {
	p, id, err := ParseRoom(p, id)
	if err != nil {
		return Room{}, err
	}
	room := Room{Platform: p, ID: id}
	if p == Bilibili {
		var v struct {
			Code int `json:"code"`
			Data struct {
				ID       int64  `json:"room_id"`
				Title    string `json:"title"`
				Live     int    `json:"live_status"`
				Cover    string `json:"user_cover"`
				Keyframe string `json:"keyframe"`
				Category string `json:"area_name"`
			} `json:"data"`
		}
		err = c.request(ctx, p, http.MethodGet, "https://api.live.bilibili.com/room/v1/Room/get_info?room_id="+id, "https://live.bilibili.com/", nil, &v)
		if err != nil {
			return Room{}, err
		}
		if v.Code != 0 || v.Data.ID == 0 {
			return Room{}, fmt.Errorf("未找到 B 站直播间")
		}
		room.ID = strconv.FormatInt(v.Data.ID, 10)
		room.Title = v.Data.Title
		room.Live = v.Data.Live == 1
		room.Cover = v.Data.Cover
		room.Category = v.Data.Category
		if room.Cover == "" {
			room.Cover = v.Data.Keyframe
		}
	} else {
		var v struct {
			Room struct {
				ID     json.Number `json:"room_id"`
				Title  string      `json:"room_name"`
				Anchor string      `json:"nickname"`
				Cover  string      `json:"room_pic"`
				Live   int         `json:"show_status"`
				Loop   int         `json:"videoLoop"`
			} `json:"room"`
		}
		err = c.request(ctx, p, http.MethodGet, "https://www.douyu.com/betard/"+id, "https://www.douyu.com/", nil, &v)
		if err != nil {
			return Room{}, err
		}
		if v.Room.ID == "" {
			return Room{}, fmt.Errorf("未找到斗鱼直播间")
		}
		room.ID = string(v.Room.ID)
		room.Title = v.Room.Title
		room.Anchor = v.Room.Anchor
		room.Cover = v.Room.Cover
		room.Live = v.Room.Live == 1 && v.Room.Loop != 1
	}
	return room, nil
}
