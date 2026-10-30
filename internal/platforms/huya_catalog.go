package platforms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

func (c *Client) huyaCategories(ctx context.Context) ([]Category, error) {
	var out []Category
	for _, parent := range []Category{{ID: "1", Name: "网游"}, {ID: "2", Name: "单机"}, {ID: "3", Name: "手游"}, {ID: "8", Name: "娱乐"}} {
		var r struct {
			Data []struct {
				ID   json.RawMessage `json:"gid"`
				Name string          `json:"gameFullName"`
			} `json:"data"`
		}
		if err := c.request(ctx, Huya, http.MethodGet, "https://live.cdn.huya.com/liveconfig/game/bussLive?bussType="+parent.ID, "https://www.huya.com/", nil, &r); err != nil {
			return nil, err
		}
		for _, v := range r.Data {
			var id json.Number
			if json.Unmarshal(v.ID, &id) != nil {
				var obj struct {
					Value json.Number `json:"value"`
				}
				if json.Unmarshal(v.ID, &obj) == nil {
					id = obj.Value
				}
			}
			if id != "" {
				parent.Children = append(parent.Children, Category{ID: string(id), ParentID: parent.ID, Name: v.Name})
			}
		}
		out = append(out, parent)
	}
	return out, nil
}
func (c *Client) huyaSearch(ctx context.Context, query string, page int) (Catalog, error) {
	q := url.Values{"m": {"Search"}, "do": {"getSearchContent"}, "q": {query}, "uid": {"0"}, "v": {"4"}, "typ": {"-5"}, "livestate": {"0"}, "rows": {"30"}, "start": {strconv.Itoa((max(page, 1) - 1) * 30)}}
	var r struct {
		Response struct {
			Rooms struct {
				Docs []struct {
					ID     json.Number `json:"room_id"`
					Title  string      `json:"game_introduction"`
					Anchor string      `json:"game_nick"`
					Cover  string      `json:"game_screenshot"`
				} `json:"docs"`
				Found int `json:"numFound"`
			} `json:"3"`
		} `json:"response"`
	}
	if err := c.request(ctx, Huya, http.MethodGet, "https://search.cdn.huya.com/?"+q.Encode(), "https://www.huya.com/", nil, &r); err != nil {
		return Catalog{}, err
	}
	result := r.Response.Rooms
	out := Catalog{HasMore: page*30 < result.Found}
	for _, v := range result.Docs {
		if v.ID != "" {
			out.Rooms = append(out.Rooms, Room{Platform: Huya, ID: string(v.ID), Title: v.Title, Anchor: v.Anchor, Cover: v.Cover, Live: true})
		}
	}
	return out, nil
}
