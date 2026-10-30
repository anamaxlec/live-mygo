package platforms

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

const douyinUA = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36 Core/1.116.567.400 QQBrowser/19.7.6764.400"

type douyinImage struct {
	URLs []string `json:"url_list"`
}

func (v douyinImage) first() string {
	if len(v.URLs) > 0 {
		return v.URLs[0]
	}
	return ""
}

type douyinRoom struct {
	ID     string      `json:"id_str"`
	Title  string      `json:"title"`
	Status int         `json:"status"`
	Cover  douyinImage `json:"cover"`
	Owner  struct {
		Nick  string `json:"nickname"`
		WebID string `json:"web_rid"`
	} `json:"owner"`
	Stream struct {
		FLV  map[string]string `json:"flv_pull_url"`
		HLS  map[string]string `json:"hls_pull_url_map"`
		Core struct {
			Pull struct {
				Data    string `json:"stream_data"`
				Options struct {
					Qualities []struct {
						Name  string `json:"name"`
						Key   string `json:"sdk_key"`
						Level int    `json:"level"`
					} `json:"qualities"`
				} `json:"options"`
			} `json:"pull_data"`
		} `json:"live_core_sdk_data"`
	} `json:"stream_url"`
}
type douyinPageState struct {
	RoomStore struct {
		Info struct {
			Room   douyinRoom `json:"room"`
			ID     string     `json:"roomId"`
			WebID  string     `json:"web_rid"`
			Anchor struct {
				Nick string `json:"nickname"`
			} `json:"anchor"`
		} `json:"roomInfo"`
	} `json:"roomStore"`
	UserStore struct {
		Odin struct {
			ID string `json:"user_unique_id"`
		} `json:"odin"`
		User struct {
			ID string `json:"user_unique_id"`
		} `json:"user"`
	} `json:"userStore"`
}

func (r douyinRoom) room(id string) Room {
	return Room{Platform: Douyin, ID: id, Title: r.Title, Anchor: r.Owner.Nick, Cover: r.Cover.first(), Live: r.Status == 2}
}

func (c *Client) douyinCookie(ctx context.Context) (string, error) {
	saved := ""
	if c.Cookies != nil {
		var err error
		saved, err = c.Cookies.Header(Douyin, "https://live.douyin.com/")
		if err != nil {
			return "", err
		}
		if strings.Contains(saved, "ttwid=") {
			return saved, nil
		}
	}
	c.visitorMu.Lock()
	defer c.visitorMu.Unlock()
	if c.douyinVisitor == "" {
		body := `{"region":"cn","aid":1768,"needFid":false,"service":"www.ixigua.com","migrate_info":{"ticket":"","source":"node"},"cbUrlProtocol":"https","union":true}`
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://ttwid.bytedance.com/ttwid/union/register/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", douyinUA)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return "", err
		}
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "ttwid" {
				c.douyinVisitor = cookie.Name + "=" + cookie.Value
			}
		}
		resp.Body.Close()
		if c.douyinVisitor == "" {
			return "", errors.New("抖音匿名会话初始化失败")
		}
	}
	if saved != "" {
		return saved + "; " + c.douyinVisitor, nil
	}
	return c.douyinVisitor, nil
}
func (c *Client) douyinPage(ctx context.Context, id string) (douyinPageState, string, error) {
	var state douyinPageState
	cookie, err := c.douyinCookie(ctx)
	if err != nil {
		return state, "", err
	}
	html, err := c.page(ctx, "https://live.douyin.com/"+url.PathEscape(id), "https://live.douyin.com/", douyinUA, cookie)
	if err != nil {
		return state, "", err
	}
	if err = flightValue(html, "state", &state); err != nil {
		return state, "", fmt.Errorf("抖音房间资料不可用，请稍后重试")
	}
	if state.RoomStore.Info.ID == "" {
		return state, "", fmt.Errorf("未找到抖音直播间")
	}
	if state.RoomStore.Info.Room.Owner.Nick == "" {
		state.RoomStore.Info.Room.Owner.Nick = state.RoomStore.Info.Anchor.Nick
	}
	return state, cookie, nil
}
func (c *Client) douyin(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	state, _, err := c.douyinPage(ctx, id)
	if err != nil {
		return nil, err
	}
	r := state.RoomStore.Info.Room
	if r.Status != 2 {
		return nil, errors.New("直播间未开播")
	}
	s := &Stream{Room: r.room(id)}
	type entry struct {
		Quality
		FLV, HLS string
	}
	var entries []entry
	var core struct {
		Data map[string]struct {
			Main struct {
				FLV string `json:"flv"`
				HLS string `json:"hls"`
			} `json:"main"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(r.Stream.Core.Pull.Data), &core) == nil {
		for _, q := range r.Stream.Core.Pull.Options.Qualities {
			line := core.Data[q.Key]
			if line.Main.FLV != "" || line.Main.HLS != "" {
				entries = append(entries, entry{Quality{q.Level, q.Name}, line.Main.FLV, line.Main.HLS})
			}
		}
	}
	if len(entries) == 0 {
		for i, key := range []string{"SD2", "SD1", "HD1", "FULL_HD1"} {
			if r.Stream.FLV[key] != "" || r.Stream.HLS[key] != "" {
				entries = append(entries, entry{Quality{i + 1, []string{"标清", "高清", "超清", "原画"}[i]}, r.Stream.FLV[key], r.Stream.HLS[key]})
			}
		}
	}
	if len(entries) == 0 {
		return nil, errors.New("抖音未返回可播放画面")
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].ID > entries[j].ID })
	selected := entries[0]
	for _, entry := range entries {
		s.Qualities = append(s.Qualities, entry.Quality)
		if entry.ID == quality {
			selected = entry
		}
	}
	s.QualityID = selected.ID
	for _, line := range []struct{ ID, URL, Format string }{{"flv", selected.FLV, "FLV"}, {"hls", selected.HLS, "HLS"}} {
		if line.URL != "" {
			s.Sources = append(s.Sources, Source{ID: line.ID, Name: line.Format, Format: line.Format, URL: line.URL, Headers: map[string]string{"User-Agent": douyinUA, "Referer": "https://live.douyin.com/"}})
		}
	}
	if len(route) == 0 || !s.SelectSource(route[0]) {
		s.SelectSource(s.Sources[0].ID)
	}
	return s, nil
}
func (c *Client) douyinCatalog(ctx context.Context, page int) (Catalog, error) {
	cookie, err := c.douyinCookie(ctx)
	if err != nil {
		return Catalog{}, err
	}
	q := url.Values{"aid": {"6383"}, "app_name": {"douyin_web"}, "need_map": {"1"}, "is_draw": {"1"}, "inner_from_drawer": {"0"}, "enter_source": {"web_homepage_hot_web_live_card"}, "source_key": {"web_homepage_hot_web_live_card"}}
	data, err := c.page(ctx, "https://live.douyin.com/webcast/feed/?"+q.Encode(), "https://live.douyin.com/", douyinUA, cookie)
	if err != nil {
		return Catalog{}, err
	}
	var response struct {
		Data []struct {
			WebID string     `json:"web_rid"`
			Room  douyinRoom `json:"data"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &response) != nil {
		return Catalog{}, errors.New("抖音推荐暂时不可用")
	}
	out := Catalog{}
	for _, item := range response.Data {
		id := item.WebID
		if id == "" {
			id = item.Room.Owner.WebID
		}
		if id != "" {
			r := item.Room.room(id)
			r.Live = true
			out.Rooms = append(out.Rooms, r)
		}
	}
	return out, nil
}
