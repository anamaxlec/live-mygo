package platforms

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const huyaUA = "HYSDK(Windows, 30000002)_APP(pc_exe&7060000&official)_SDK(trans&2.32.3.5646)"

type huyaLine struct {
	Channel int64  `json:"lChannelId"`
	Sub     int64  `json:"lSubChannelId"`
	Name    string `json:"sCdnType"`
	Stream  string `json:"sStreamName"`
	URL     string `json:"sFlvUrl"`
	Anti    string `json:"sFlvAntiCode"`
	UID     int64  `json:"lPresenterUid"`
}
type huyaProfile struct {
	Status int `json:"status"`
	Data   struct {
		Live struct {
			ID       json.Number `json:"profileRoom"`
			Title    string      `json:"introduction"`
			Anchor   string      `json:"nick"`
			Cover    string      `json:"screenshot"`
			Category string      `json:"gameFullName"`
			UID      int64       `json:"uid"`
			BitRates string      `json:"bitRateInfo"`
		} `json:"liveData"`
		Stream struct {
			Lines []huyaLine `json:"baseSteamInfoList"`
		} `json:"stream"`
		LiveStatus string `json:"liveStatus"`
	} `json:"data"`
}

func (c *Client) huyaProfile(ctx context.Context, id string) (huyaProfile, error) {
	var r huyaProfile
	err := c.request(ctx, Huya, http.MethodGet, "https://mp.huya.com/cache.php?m=Live&do=profileRoom&roomid="+url.QueryEscape(id), "https://www.huya.com/", nil, &r)
	if err == nil && (r.Status != 200 || r.Data.Live.ID == "") {
		err = fmt.Errorf("未找到虎牙直播间")
	}
	return r, err
}
func (r huyaProfile) room() Room {
	v := r.Data.Live
	return Room{Platform: Huya, ID: string(v.ID), Title: v.Title, Anchor: v.Anchor, Cover: v.Cover, Category: v.Category, Live: len(r.Data.Stream.Lines) > 0 && r.Data.LiveStatus != "OFF"}
}
func (c *Client) huya(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	profile, err := c.huyaProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	if !profile.room().Live {
		return nil, fmt.Errorf("直播间未开播")
	}
	s := &Stream{Room: profile.room()}
	var rates []struct {
		Name string `json:"sDisplayName"`
		ID   int    `json:"iBitRate"`
	}
	if err = json.Unmarshal([]byte(profile.Data.Live.BitRates), &rates); err != nil {
		return nil, fmt.Errorf("虎牙未返回清晰度信息")
	}
	for _, q := range rates {
		if !strings.Contains(q.Name, "HDR") {
			s.Qualities = append(s.Qualities, Quality{ID: q.ID, Name: q.Name})
		}
	}
	if len(s.Qualities) == 0 {
		return nil, fmt.Errorf("虎牙未返回可用清晰度")
	}
	s.QualityID = s.Qualities[0].ID
	for _, q := range s.Qualities {
		if q.ID == quality {
			s.QualityID = quality
		}
	}
	tokens := map[string]string{}
	for _, line := range profile.Data.Stream.Lines {
		if line.URL == "" || line.Stream == "" {
			continue
		}
		anti := tokens[line.Stream]
		if anti == "" {
			anti, err = c.huyaToken(ctx, line.Stream)
			if err != nil {
				return nil, err
			}
			tokens[line.Stream] = anti
		}
		query, err := huyaAnti(line.Stream, line.UID, anti, time.Now())
		if err != nil {
			return nil, err
		}
		if s.QualityID > 0 {
			query.Set("ratio", strconv.Itoa(s.QualityID))
		}
		query.Set("codec", "264")
		raw := strings.Replace(line.URL, "http:", "https:", 1) + "/" + line.Stream + ".flv?" + query.Encode()
		s.Sources = append(s.Sources, Source{ID: line.Name, Name: "线路 " + strconv.Itoa(len(s.Sources)+1), Format: "FLV", URL: raw, Headers: map[string]string{"User-Agent": huyaUA, "Referer": "https://www.huya.com/"}})
	}
	if len(s.Sources) == 0 {
		return nil, fmt.Errorf("虎牙未返回可播放线路")
	}
	selected := s.Sources[0].ID
	if len(route) > 0 && route[0] != "" {
		selected = route[0]
	}
	if !s.SelectSource(selected) {
		s.SelectSource(s.Sources[0].ID)
	}
	return s, nil
}
func huyaAnti(stream string, uid int64, anti string, now time.Time) (url.Values, error) {
	q, err := url.ParseQuery(anti)
	if err != nil {
		return nil, err
	}
	fm, err := base64.StdEncoding.DecodeString(q.Get("fm"))
	if err != nil || len(fm) == 0 || q.Get("wsTime") == "" {
		return nil, fmt.Errorf("虎牙线路凭据无效")
	}
	t := q.Get("t")
	calc := uint64(uid)
	if t != "103" {
		calc = calc<<8 | calc>>56
	}
	seq := strconv.FormatInt(uid+now.UnixMilli(), 10)
	hash := func(s string) string { return fmt.Sprintf("%x", md5.Sum([]byte(s))) }
	secret := hash(fmt.Sprintf("%s_%d_%s_%s_%s", strings.Split(string(fm), "_")[0], calc, stream, hash(seq+"|"+q.Get("ctype")+"|"+t), q.Get("wsTime")))
	out := url.Values{"wsSecret": {secret}, "wsTime": {q.Get("wsTime")}, "seqid": {seq}, "ctype": {q.Get("ctype")}, "ver": {"1"}, "fs": {q.Get("fs")}, "t": {t}}
	if t == "103" {
		out.Set("uid", strconv.FormatInt(uid, 10))
		out.Set("uuid", strconv.FormatInt(now.UnixMilli()%4294967295, 10))
	} else {
		out.Set("u", strconv.FormatUint(calc, 10))
	}
	return out, nil
}
func (c *Client) huyaCatalog(ctx context.Context, page int, category string) (Catalog, error) {
	target := fmt.Sprintf("https://www.huya.com/cache.php?m=LiveList&do=getLiveListByPage&tagAll=0&page=%d", page)
	if category != "" {
		target += "&gameId=" + url.QueryEscape(category)
	}
	var r struct {
		Status int `json:"status"`
		Data   struct {
			Page  int `json:"page"`
			Pages int `json:"totalPage"`
			Rooms []struct {
				ID       json.Number `json:"profileRoom"`
				Title    string      `json:"introduction"`
				Anchor   string      `json:"nick"`
				Cover    string      `json:"screenshot"`
				Category string      `json:"gameFullName"`
				Heat     json.Number `json:"totalCount"`
			} `json:"datas"`
		} `json:"data"`
	}
	if err := c.request(ctx, Huya, http.MethodGet, target, "https://www.huya.com/", nil, &r); err != nil {
		return Catalog{}, err
	}
	if r.Status != 200 {
		return Catalog{}, fmt.Errorf("虎牙目录加载失败")
	}
	out := Catalog{HasMore: r.Data.Page < r.Data.Pages}
	for _, v := range r.Data.Rooms {
		heat, _ := v.Heat.Int64()
		out.Rooms = append(out.Rooms, Room{Platform: Huya, ID: string(v.ID), Title: v.Title, Anchor: v.Anchor, Cover: v.Cover, Category: v.Category, Heat: heat, Live: true})
	}
	return out, nil
}
