package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type Category struct {
	ID       string     `json:"id"`
	ParentID string     `json:"parent_id,omitempty"`
	Name     string     `json:"name"`
	Children []Category `json:"children,omitempty"`
}

func (c *Client) Categories(ctx context.Context, p Platform) ([]Category, error) {
	if p == FLive {
		return nil, nil
	}
	if p == Douyin {
		return c.douyinCategories(ctx)
	}
	if p == Qie {
		return c.qieCategories(ctx)
	}
	if p == Huya {
		return c.huyaCategories(ctx)
	}
	if p == Douyu {
		var r struct {
			Code int `json:"code"`
			Data struct {
				Parents []struct {
					ID   json.Number `json:"cate1Id"`
					Name string      `json:"cate1Name"`
				} `json:"cate1Info"`
				Children []struct {
					ID     json.Number `json:"cate2Id"`
					Parent json.Number `json:"cate1Id"`
					Name   string      `json:"cate2Name"`
				} `json:"cate2Info"`
			} `json:"data"`
		}
		if err := c.request(ctx, p, http.MethodGet, "https://m.douyu.com/api/cate/list", "https://m.douyu.com/", nil, &r); err != nil {
			return nil, err
		}
		var out []Category
		for _, parent := range r.Data.Parents {
			group := Category{ID: string(parent.ID), Name: parent.Name}
			for _, sub := range r.Data.Children {
				if sub.Parent == parent.ID {
					group.Children = append(group.Children, Category{ID: string(sub.ID), ParentID: group.ID, Name: sub.Name})
				}
			}
			if len(group.Children) > 0 {
				out = append(out, group)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("斗鱼未返回直播分区")
		}
		return out, nil
	}
	var r struct {
		Code int `json:"code"`
		Data []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
			List []struct {
				ID   json.Number `json:"id"`
				Name string      `json:"name"`
			} `json:"list"`
		} `json:"data"`
	}
	if err := c.request(ctx, p, http.MethodGet, "https://api.live.bilibili.com/room/v1/Area/getList?need_entrance=1&parent_id=0", "https://live.bilibili.com/", nil, &r); err != nil {
		return nil, err
	}
	if r.Code != 0 {
		return nil, fmt.Errorf("B 站分区加载失败（code %d）", r.Code)
	}
	var out []Category
	for _, parent := range r.Data {
		group := Category{ID: string(parent.ID), Name: parent.Name}
		for _, sub := range parent.List {
			group.Children = append(group.Children, Category{ID: string(sub.ID), ParentID: group.ID, Name: sub.Name})
		}
		if len(group.Children) > 0 {
			out = append(out, group)
		}
	}
	return out, nil
}

func (c *Client) CategoryRooms(ctx context.Context, p Platform, category Category, page int) (Catalog, error) {
	if category.ID == "" {
		return c.Recommend(ctx, p, page)
	}
	if p == Douyin {
		return c.douyinCategory(ctx, category, page)
	}
	if p == Qie {
		return c.qieCatalog(ctx, category.ID)
	}
	if p == Huya {
		return c.huyaCatalog(ctx, page, category.ID)
	}
	if p == Douyu {
		out, err := c.douyuCatalog(ctx, fmt.Sprintf("https://www.douyu.com/gapi/rkc/directory/mixList/2_%s/%d", url.PathEscape(category.ID), page), page)
		return out, err
	}
	return c.biliCatalog(ctx, page, category)
}
