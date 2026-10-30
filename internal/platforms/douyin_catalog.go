package platforms

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
)

//go:embed scripts/douyin-abogus.js
var douyinABogus string
var douyinABogusProgram struct {
	sync.Once
	program *goja.Program
	err     error
}

func (c *Client) douyinSigned(ctx context.Context, target string, q url.Values, agent, cookie string) ([]byte, error) {
	douyinABogusProgram.Do(func() {
		douyinABogusProgram.program, douyinABogusProgram.err = goja.Compile("douyin-abogus.js", douyinABogus, false)
	})
	if douyinABogusProgram.err != nil {
		return nil, douyinABogusProgram.err
	}
	rt := goja.New()
	timer := time.AfterFunc(2*time.Second, func() { rt.Interrupt("signature timeout") })
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { rt.Interrupt(ctx.Err()) })
	defer stop()
	if _, err := rt.RunProgram(douyinABogusProgram.program); err != nil {
		return nil, err
	}
	fn, ok := goja.AssertFunction(rt.Get("getABogus"))
	if !ok {
		return nil, errors.New("抖音请求签名不可用")
	}
	query := q.Encode()
	sig, err := fn(goja.Undefined(), rt.ToValue(query), rt.ToValue(agent))
	if err != nil {
		return nil, err
	}
	timer.Stop()
	stop()
	return c.page(ctx, target+"?"+query+"&a_bogus="+url.QueryEscape(sig.String()), "https://live.douyin.com/", agent, cookie)
}

type douyinPartition struct {
	Partition struct {
		ID    json.Number `json:"id_str"`
		Type  int         `json:"type"`
		Title string      `json:"title"`
	} `json:"partition"`
	Children []douyinPartition `json:"sub_partition"`
}

func (p douyinPartition) category() Category {
	id := string(p.Partition.ID) + "," + strconv.Itoa(p.Partition.Type)
	result := Category{ID: id, Name: p.Partition.Title, Children: []Category{{ID: id, ParentID: id, Name: "全部" + p.Partition.Title}}}
	var walk func([]douyinPartition)
	walk = func(items []douyinPartition) {
		for _, item := range items {
			cid := string(item.Partition.ID) + "," + strconv.Itoa(item.Partition.Type)
			result.Children = append(result.Children, Category{ID: cid, ParentID: id, Name: item.Partition.Title})
			walk(item.Children)
		}
	}
	walk(p.Children)
	return result
}
func (c *Client) douyinCategories(ctx context.Context) ([]Category, error) {
	cookie, err := c.douyinCookie(ctx)
	if err != nil {
		return nil, err
	}
	page, err := c.page(ctx, "https://live.douyin.com/", "https://live.douyin.com/", douyinUA, cookie)
	if err != nil {
		return nil, err
	}
	var rows []douyinPartition
	if err = flightValue(page, "categoryData", &rows); err != nil {
		return nil, err
	}
	var out []Category
	for _, r := range rows {
		if r.Partition.ID != "" {
			out = append(out, r.category())
		}
	}
	return out, nil
}
func (c *Client) douyinCategory(ctx context.Context, category Category, page int) (Catalog, error) {
	parts := strings.SplitN(category.ID, ",", 2)
	if len(parts) != 2 {
		return Catalog{}, errors.New("抖音分类无效")
	}
	cookie, err := c.douyinCookie(ctx)
	if err != nil {
		return Catalog{}, err
	}
	q := url.Values{"aid": {"6383"}, "app_name": {"douyin_web"}, "live_id": {"1"}, "device_platform": {"web"}, "language": {"zh-CN"}, "enter_from": {"link_share"}, "cookie_enabled": {"true"}, "screen_width": {"1980"}, "screen_height": {"1080"}, "browser_language": {"zh-CN"}, "browser_platform": {"Win32"}, "browser_name": {"Edge"}, "browser_version": {"125.0.0.0"}, "browser_online": {"true"}, "count": {"15"}, "offset": {strconv.Itoa((max(1, page) - 1) * 15)}, "partition": {parts[0]}, "partition_type": {parts[1]}, "req_from": {"2"}}
	data, err := c.douyinSigned(ctx, "https://live.douyin.com/webcast/web/partition/detail/room/v2/", q, douyinUA, cookie)
	if err != nil {
		return Catalog{}, err
	}
	var result struct {
		Code int `json:"status_code"`
		Data struct {
			Rooms []struct {
				WebID string     `json:"web_rid"`
				Room  douyinRoom `json:"room"`
			} `json:"data"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil {
		return Catalog{}, errors.New("抖音分类返回了不可解析的数据")
	}
	if result.Code != 0 {
		return Catalog{}, fmt.Errorf("抖音分类请求失败（code %d）", result.Code)
	}
	out := Catalog{HasMore: len(result.Data.Rooms) >= 15}
	for _, entry := range result.Data.Rooms {
		id := entry.WebID
		if id == "" {
			id = entry.Room.Owner.WebID
		}
		if id != "" {
			out.Rooms = append(out.Rooms, entry.Room.room(id))
		}
	}
	return out, nil
}

const douyinSearchUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36 Edg/125.0.0.0"

func (c *Client) douyinSearch(ctx context.Context, query string, page int) (Catalog, error) {
	cookie, err := c.douyinCookie(ctx)
	if err != nil {
		return Catalog{}, err
	}
	q := url.Values{"device_platform": {"webapp"}, "aid": {"6383"}, "channel": {"channel_pc_web"}, "search_channel": {"aweme_live"}, "keyword": {query}, "search_source": {"switch_tab"}, "query_correct_type": {"1"}, "is_filter_search": {"0"}, "from_group_id": {""}, "offset": {strconv.Itoa((max(1, page) - 1) * 10)}, "count": {"10"}, "need_filter_settings": {"1"}, "list_type": {"single"}, "pc_client_type": {"1"}, "version_code": {"170400"}, "version_name": {"17.4.0"}, "update_version_code": {"170400"}, "cookie_enabled": {"true"}, "screen_width": {"1980"}, "screen_height": {"1080"}, "browser_language": {"zh-CN"}, "browser_platform": {"Win32"}, "browser_name": {"Edge"}, "browser_version": {"125.0.0.0"}, "browser_online": {"true"}, "engine_name": {"Blink"}, "engine_version": {"125.0.0.0"}, "os_name": {"Windows"}, "os_version": {"10"}, "cpu_core_num": {"12"}, "device_memory": {"8"}, "platform": {"PC"}, "downlink": {"10"}, "effective_type": {"4g"}, "round_trip_time": {"100"}}
	data, err := c.douyinSigned(ctx, "https://www.douyin.com/aweme/v1/web/live/search/", q, douyinSearchUA, cookie)
	if err != nil {
		return Catalog{}, err
	}
	var result struct {
		Code int `json:"status_code"`
		More int `json:"has_more"`
		Data []struct {
			Lives struct {
				Raw string `json:"rawdata"`
			} `json:"lives"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &result) != nil {
		return Catalog{}, errors.New("抖音搜索暂时受限，请在账号设置登录后重试")
	}
	if result.Code != 0 {
		return Catalog{}, fmt.Errorf("抖音搜索需要有效登录会话（code %d），可在账号设置中登录", result.Code)
	}
	out := Catalog{HasMore: result.More > 0}
	for _, entry := range result.Data {
		var r douyinRoom
		if json.Unmarshal([]byte(entry.Lives.Raw), &r) == nil && r.Owner.WebID != "" {
			out.Rooms = append(out.Rooms, r.room(r.Owner.WebID))
		}
	}
	return out, nil
}
