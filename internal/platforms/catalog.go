package platforms

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Catalog struct {
	Rooms   []Room `json:"rooms"`
	HasMore bool   `json:"has_more"`
}

func (c *Client) Recommend(ctx context.Context, p Platform, page int) (Catalog, error) {
	if p == FLive {
		return c.fliveCatalog(ctx, "")
	}
	if p == Douyin {
		return c.douyinCatalog(ctx, page)
	}
	if p == Qie {
		return c.qieCatalog(ctx, "")
	}
	if p == Huya {
		return c.huyaCatalog(ctx, page, "")
	}
	if p == Douyu {
		return c.douyuCatalog(ctx, fmt.Sprintf("https://www.douyu.com/japi/weblist/apinc/allpage/6/%d", page), page)
	}
	return c.biliCatalog(ctx, page, Category{})
}

func (c *Client) douyuCatalog(ctx context.Context, target string, page int) (Catalog, error) {
	var r struct {
		Error int `json:"error"`
		Data  struct {
			Pages int `json:"pgcnt"`
			List  []struct {
				ID       json.Number `json:"rid"`
				Title    string      `json:"rn"`
				Name     string      `json:"nn"`
				Cover    string      `json:"rs16"`
				Type     int         `json:"type"`
				Heat     int64       `json:"ol"`
				Category string      `json:"c2name"`
			} `json:"rl"`
		} `json:"data"`
	}
	if err := c.request(ctx, Douyu, http.MethodGet, target, "https://www.douyu.com/", nil, &r); err != nil {
		return Catalog{}, err
	}
	if r.Error != 0 {
		return Catalog{}, fmt.Errorf("斗鱼推荐请求失败（code %d）", r.Error)
	}
	out := Catalog{HasMore: page < r.Data.Pages}
	for _, v := range r.Data.List {
		if v.Type == 1 {
			out.Rooms = append(out.Rooms, Room{Platform: Douyu, ID: string(v.ID), Title: v.Title, Anchor: v.Name, Cover: v.Cover, Live: true, Heat: v.Heat, Category: v.Category})
		}
	}
	return out, nil
}

func (c *Client) biliCatalog(ctx context.Context, page int, category Category) (Catalog, error) {
	session, err := c.biliGuest(ctx)
	if err != nil {
		return Catalog{}, err
	}
	c = session
	q := url.Values{"platform": {"web"}, "sort": {"online"}, "page_size": {"24"}, "page": {strconv.Itoa(page)}}
	if category.ID != "" {
		q.Set("parent_area_id", category.ParentID)
		q.Set("area_id", category.ID)
	}
	if err := c.wbi(ctx, q); err != nil {
		return Catalog{}, err
	}
	var r struct {
		Code int `json:"code"`
		Data struct {
			List []biliCard `json:"list"`
		} `json:"data"`
	}
	if err := c.request(ctx, Bilibili, http.MethodGet, "https://api.live.bilibili.com/xlive/web-interface/v1/second/getListByArea?"+q.Encode(), "https://live.bilibili.com/", nil, &r); err != nil {
		return Catalog{}, err
	}
	if r.Code != 0 {
		return Catalog{}, fmt.Errorf("B 站推荐请求失败（code %d），可在账号设置中登录后重试", r.Code)
	}
	out := Catalog{HasMore: len(r.Data.List) >= 24}
	for _, v := range r.Data.List {
		out.Rooms = append(out.Rooms, v.room())
	}
	return out, nil
}

type biliCard struct {
	ID       json.Number `json:"roomid"`
	Title    string      `json:"title"`
	Name     string      `json:"uname"`
	Cover    string      `json:"cover"`
	Category string      `json:"area_name"`
	Heat     int64       `json:"online"`
}

var highlight = regexp.MustCompile(`<[^>]*>`)

func (v biliCard) room() Room {
	cover := v.Cover
	if strings.HasPrefix(cover, "//") {
		cover = "https:" + cover
	}
	return Room{Platform: Bilibili, ID: string(v.ID), Title: html.UnescapeString(highlight.ReplaceAllString(v.Title, "")), Anchor: v.Name, Cover: cover, Live: true, Heat: v.Heat, Category: v.Category}
}

func (c *Client) Search(ctx context.Context, p Platform, query string, page int) (Catalog, error) {
	if p == FLive {
		return c.fliveCatalog(ctx, query)
	}
	if p == Douyin {
		return c.douyinSearch(ctx, query, page)
	}
	if p == Qie {
		return c.qieSearch(ctx, query, page)
	}
	if p == Huya {
		return c.huyaSearch(ctx, query, page)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return Catalog{}, errors.New("请输入主播或直播间名称")
	}
	q := url.Values{"page": {strconv.Itoa(page)}}
	if p == Douyu {
		q.Set("kw", query)
		q.Set("pageSize", "20")
		var r struct {
			Error int `json:"error"`
			Data  struct {
				List []struct {
					ID    json.Number `json:"rid"`
					Title string      `json:"roomName"`
					Name  string      `json:"nickName"`
					Cover string      `json:"roomSrc"`
				} `json:"relateShow"`
			} `json:"data"`
		}
		if err := c.request(ctx, p, http.MethodGet, "https://www.douyu.com/japi/search/api/searchShow?"+q.Encode(), "https://www.douyu.com/search/", nil, &r); err != nil {
			return Catalog{}, err
		}
		if r.Error != 0 {
			return Catalog{}, fmt.Errorf("斗鱼搜索失败（code %d）", r.Error)
		}
		out := Catalog{HasMore: len(r.Data.List) >= 20}
		for _, v := range r.Data.List {
			out.Rooms = append(out.Rooms, Room{Platform: p, ID: string(v.ID), Title: html.UnescapeString(highlight.ReplaceAllString(v.Title, "")), Anchor: v.Name, Cover: v.Cover, Live: true})
		}
		return out, nil
	}
	q.Set("search_type", "live")
	session, err := c.biliGuest(ctx)
	if err != nil {
		return Catalog{}, err
	}
	c = session
	q.Set("keyword", query)
	q.Set("cover_type", "user_cover")
	q.Set("highlight", "0")
	var r struct {
		Code int `json:"code"`
		Data struct {
			Result struct {
				List []biliCard `json:"live_room"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := c.request(ctx, p, http.MethodGet, "https://api.bilibili.com/x/web-interface/search/type?"+q.Encode(), "https://live.bilibili.com/", nil, &r); err != nil {
		return Catalog{}, err
	}
	if r.Code != 0 {
		return Catalog{}, fmt.Errorf("B 站搜索失败（code %d），可登录后重试", r.Code)
	}
	out := Catalog{HasMore: len(r.Data.Result.List) >= 40}
	for _, v := range r.Data.Result.List {
		out.Rooms = append(out.Rooms, v.room())
	}
	return out, nil
}

func (c *Client) biliGuest(ctx context.Context) (*Client, error) {
	if c.Cookies != nil {
		header, err := c.Cookies.Header(Bilibili, "https://api.bilibili.com/")
		if err != nil {
			return nil, err
		}
		if strings.Contains(header, "buvid3=") {
			return c, nil
		}
	}
	var response struct {
		Code int `json:"code"`
		Data struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		} `json:"data"`
	}
	if err := c.request(ctx, Bilibili, http.MethodGet, "https://api.bilibili.com/x/frontend/finger/spi", "https://live.bilibili.com/", nil, &response); err != nil {
		return nil, err
	}
	if response.Code != 0 || response.Data.B3 == "" {
		return nil, errors.New("B 站游客会话初始化失败")
	}
	guest := &Client{HTTP: c.HTTP, Cookies: c.Cookies, guestBili: "buvid3=" + response.Data.B3 + "; buvid4=" + response.Data.B4}
	return guest, nil
}

func (c *Client) wbi(ctx context.Context, q url.Values) error {
	var nav struct {
		Code int `json:"code"`
		Data struct {
			Images struct {
				Image string `json:"img_url"`
				Sub   string `json:"sub_url"`
			} `json:"wbi_img"`
		} `json:"data"`
	}
	if err := c.request(ctx, Bilibili, http.MethodGet, "https://api.bilibili.com/x/web-interface/nav", "https://www.bilibili.com/", nil, &nav); err != nil {
		return err
	}
	key := func(raw string) string {
		u, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		base := path.Base(u.Path)
		return strings.TrimSuffix(base, path.Ext(base))
	}
	source := key(nav.Data.Images.Image) + key(nav.Data.Images.Sub)
	if len(source) < 64 {
		return errors.New("B 站未返回请求签名参数")
	}
	table := []int{46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13, 37, 48, 7, 16, 24, 55, 40, 61, 26, 17, 0, 1, 60, 51, 30, 4, 22, 25, 54, 21, 56, 59, 6, 63, 57, 62, 11, 36, 20, 34, 44, 52}
	mixin := make([]byte, 32)
	for i, n := range table[:32] {
		mixin[i] = source[n]
	}
	q.Set("wts", strconv.FormatInt(time.Now().Unix(), 10))
	for k, values := range q {
		for i, v := range values {
			values[i] = strings.Map(func(r rune) rune {
				if strings.ContainsRune("!'()*", r) {
					return -1
				}
				return r
			}, v)
		}
		q[k] = values
	}
	canonical := strings.ReplaceAll(q.Encode(), "+", "%20")
	sum := md5.Sum([]byte(canonical + string(mixin)))
	q.Set("w_rid", hex.EncodeToString(sum[:]))
	return nil
}
