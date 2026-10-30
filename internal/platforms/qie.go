package platforms

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type qieRoom struct {
	ID     json.Number `json:"room_id"`
	URL    string      `json:"url"`
	Title  string      `json:"room_name"`
	Nick   string      `json:"nickname"`
	Cover  string      `json:"room_src"`
	Game   string      `json:"game_name"`
	Live   json.Number `json:"show_status"`
	IsLive json.Number `json:"is_live"`
}

func (r qieRoom) room() Room {
	id := string(r.ID)
	if id == "" {
		id = strings.Trim(r.URL, "/")
	}
	return Room{Platform: Qie, ID: id, Title: r.Title, Anchor: r.Nick, Cover: strings.Replace(r.Cover, "http:", "https:", 1), Category: r.Game, Live: r.Live == "1" || r.IsLive == "1"}
}

type qiePageState struct {
	Props struct {
		Initial struct {
			Home struct {
				Data struct {
					Rooms    []qieRoom `json:"live_room"`
					Channels []struct {
						ID    string    `json:"shortname"`
						Title string    `json:"title"`
						Rooms []qieRoom `json:"roomlist"`
					} `json:"channel_list"`
				} `json:"data"`
			} `json:"homeInfo"`
			Room struct {
				Info struct {
					Room qieRoom `json:"room_info"`
				} `json:"roomInfo"`
			} `json:"roomInfo"`
		} `json:"initialState"`
	} `json:"props"`
}

func (c *Client) qiePage(ctx context.Context, p string) (qiePageState, error) {
	var state qiePageState
	target := "https://www.qie.tv" + p
	cookie := ""
	if c.Cookies != nil {
		var err error
		cookie, err = c.Cookies.Header(Qie, target)
		if err != nil {
			return state, fmt.Errorf("读取本地账号失败：%w", err)
		}
	}
	data, err := c.page(ctx, target, "https://www.qie.tv/", UserAgent, cookie)
	if err != nil {
		return state, err
	}
	match := nextData.FindSubmatch(data)
	if len(match) < 2 || json.Unmarshal(match[1], &state) != nil {
		return state, errors.New("企鹅体育网页资料不可用")
	}
	return state, nil
}
func (c *Client) qieCatalog(ctx context.Context, category string) (Catalog, error) {
	state, err := c.qiePage(ctx, "/")
	if err != nil {
		return Catalog{}, err
	}
	rooms := state.Props.Initial.Home.Data.Rooms
	if category != "" {
		rooms = nil
		for _, ch := range state.Props.Initial.Home.Data.Channels {
			if ch.ID == category {
				rooms = ch.Rooms
				break
			}
		}
	}
	out := Catalog{}
	for _, r := range rooms {
		item := r.room()
		if item.ID != "" {
			out.Rooms = append(out.Rooms, item)
		}
	}
	return out, nil
}
func (c *Client) qieCategories(ctx context.Context) ([]Category, error) {
	state, err := c.qiePage(ctx, "/")
	if err != nil {
		return nil, err
	}
	var out []Category
	for _, ch := range state.Props.Initial.Home.Data.Channels {
		out = append(out, Category{ID: ch.ID, Name: ch.Title, Children: []Category{{ID: ch.ID, ParentID: ch.ID, Name: ch.Title + "推荐"}}})
	}
	return out, nil
}
func qieSignature(id, minute string) string {
	return fmt.Sprintf("%x", md5.Sum([]byte("aWR6ZWly"+id+"RgP7DW01naDYQSV0"+minute)))
}

func (c *Client) qieSearch(ctx context.Context, query string, page int) (Catalog, error) {
	var response struct {
		Rooms   []qieRoom `json:"room"`
		Anchors []qieRoom `json:"anchor"`
	}
	err := c.request(ctx, Qie, http.MethodPost, "https://www.qie.tv/api/info/search?city_code=", "https://www.qie.tv/", url.Values{"keyword": {strings.TrimSpace(query)}, "from": {"room"}}, &response)
	if err != nil {
		return Catalog{}, err
	}
	out := Catalog{}
	seen := map[string]bool{}
	for _, r := range append(response.Rooms, response.Anchors...) {
		item := r.room()
		if item.ID != "" && !seen[item.ID] {
			out.Rooms = append(out.Rooms, item)
			seen[item.ID] = true
		}
	}
	return out, nil
}
func (c *Client) qie(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	state, err := c.qiePage(ctx, "/"+url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	room := state.Props.Initial.Room.Info.Room.room()
	room.ID = id
	if !room.Live {
		return nil, errors.New("直播间未开播")
	}
	cdn := "ws"
	if len(route) > 0 && route[0] != "" {
		cdn = route[0]
	}
	minute := strconv.FormatInt((time.Now().Unix()+59)/60, 10)
	q := url.Values{"cdn": {cdn}, "nofan": {"yes"}, "_t": {minute}, "sign": {qieSignature(id, minute)}}
	var response struct {
		Error   int    `json:"error"`
		Message string `json:"msg"`
		Data    struct {
			Host   string          `json:"rtmp_url"`
			Stream string          `json:"rtmp_live"`
			CDN    string          `json:"rtmp_cdn"`
			CDNs   []string        `json:"cdns"`
			Rates  json.RawMessage `json:"rtmp_multi_bitrate"`
		} `json:"data"`
	}
	if err := c.request(ctx, Qie, http.MethodGet, "https://www.qie.tv/swf_api/web_room/"+id+"?"+q.Encode(), "https://www.qie.tv/"+id, nil, &response); err != nil {
		return nil, err
	}
	if response.Error != 0 {
		return nil, fmt.Errorf("企鹅体育取流失败（code %d）", response.Error)
	}
	if response.Data.Host == "" || response.Data.Stream == "" {
		return nil, errors.New("该直播间暂未提供播放地址")
	}
	variants := qieVariants(response.Data.Stream, response.Data.Rates)
	s := &Stream{Room: room}
	names := []string{"原画", "超清", "高清", "标清"}
	for i := range variants {
		name := fmt.Sprintf("清晰度 %d", i+1)
		if i < len(names) {
			name = names[i]
		}
		s.Qualities = append(s.Qualities, Quality{ID: i, Name: name})
	}
	if quality >= 0 && quality < len(variants) {
		s.QualityID = quality
	}
	// 每个档位使用接口返回的实际文件名，不通过改写地址猜测高清源。
	raw := strings.TrimRight(response.Data.Host, "/") + "/" + variants[s.QualityID]
	headers := map[string]string{"User-Agent": UserAgent, "Referer": "https://www.qie.tv/" + id}
	for _, id := range response.Data.CDNs {
		s.Sources = append(s.Sources, Source{ID: id, Name: "线路 " + strconv.Itoa(len(s.Sources)+1), Format: "FLV", Headers: headers})
	}
	if len(s.Sources) == 0 {
		s.Sources = []Source{{ID: cdn, Name: "自动线路", Format: "FLV", Headers: headers}}
	}
	selected := response.Data.CDN
	if selected == "" {
		selected = cdn
	}
	for i := range s.Sources {
		if s.Sources[i].ID == selected {
			s.Sources[i].URL = raw
		}
	}
	if !s.SelectSource(selected) {
		s.Sources[0].URL = raw
		s.SelectSource(s.Sources[0].ID)
	}
	return s, nil
}

func qieVariants(primary string, data json.RawMessage) []string {
	out := []string{primary}
	var values []string
	if json.Unmarshal(data, &values) != nil {
		var byName map[string]string
		if json.Unmarshal(data, &byName) == nil {
			keys := make([]string, 0, len(byName))
			for key := range byName {
				keys = append(keys, key)
			}
			sort.Slice(keys, func(i, j int) bool {
				a, ae := strconv.Atoi(keys[i])
				b, be := strconv.Atoi(keys[j])
				if ae == nil && be == nil {
					return a < b
				}
				return keys[i] < keys[j]
			})
			for _, key := range keys {
				values = append(values, byName[key])
			}
		}
	}
	seen := map[string]bool{primary: true}
	for _, value := range values {
		if value != "" && !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	return out
}
