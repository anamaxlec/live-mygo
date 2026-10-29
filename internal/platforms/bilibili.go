package platforms

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type biliCodec struct {
	Name     string `json:"codec_name"`
	Current  int    `json:"current_qn"`
	Accepted []int  `json:"accept_qn"`
	Base     string `json:"base_url"`
	URLs     []struct {
		Host  string `json:"host"`
		Extra string `json:"extra"`
	} `json:"url_info"`
}
type biliPlay struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		LiveStatus  int `json:"live_status"`
		PlayURLInfo struct {
			PlayURL struct {
				Descriptions []struct {
					ID   int    `json:"qn"`
					Name string `json:"desc"`
				} `json:"g_qn_desc"`
				Streams []struct {
					Formats []struct {
						Name   string      `json:"format_name"`
						Codecs []biliCodec `json:"codec"`
					} `json:"format"`
				} `json:"stream"`
			} `json:"playurl"`
		} `json:"playurl_info"`
	} `json:"data"`
}

func (c *Client) bilibili(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	ref := "https://live.bilibili.com/" + id
	var detail struct {
		Code int `json:"code"`
		Data struct {
			ID       int64  `json:"room_id"`
			Title    string `json:"title"`
			Status   int    `json:"live_status"`
			UID      int64  `json:"uid"`
			Cover    string `json:"user_cover"`
			Keyframe string `json:"keyframe"`
			Category string `json:"area_name"`
		} `json:"data"`
	}
	if err := c.request(ctx, Bilibili, http.MethodGet, "https://api.live.bilibili.com/room/v1/Room/get_info?room_id="+id, ref, nil, &detail); err != nil {
		return nil, err
	}
	if detail.Code != 0 {
		return nil, fmt.Errorf("B 站房间信息请求失败（code %d）", detail.Code)
	}
	if detail.Data.ID == 0 {
		return nil, errors.New("未找到 B 站直播间")
	}
	id = strconv.FormatInt(detail.Data.ID, 10)
	if detail.Data.Status != 1 {
		return nil, errors.New("该 B 站房间当前未开播")
	}
	if quality < 0 {
		quality = 10000
	}
	q := url.Values{"room_id": {id}, "protocol": {"0,1"}, "format": {"0,1,2"}, "codec": {"0"}, "platform": {"web"}, "qn": {strconv.Itoa(quality)}, "ptype": {"8"}}
	var play biliPlay
	if err := c.request(ctx, Bilibili, http.MethodGet, "https://api.live.bilibili.com/xlive/web-room/v2/index/getRoomPlayInfo?"+q.Encode(), ref, nil, &play); err != nil {
		return nil, err
	}
	if play.Code != 0 {
		return nil, fmt.Errorf("B 站取流失败（code %d）", play.Code)
	}
	stream, err := parseBiliPlay(play)
	if err != nil {
		return nil, err
	}
	cover := detail.Data.Cover
	if cover == "" {
		cover = detail.Data.Keyframe
	}
	stream.Room = Room{Platform: Bilibili, ID: id, Title: detail.Data.Title, Live: true, Cover: cover, Category: detail.Data.Category}
	if detail.Data.UID > 0 {
		metaCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		var owner struct {
			Data struct {
				Info struct {
					Name string `json:"uname"`
				} `json:"info"`
			} `json:"data"`
		}
		if c.request(metaCtx, Bilibili, http.MethodGet, "https://api.live.bilibili.com/live_user/v1/Master/info?uid="+strconv.FormatInt(detail.Data.UID, 10), ref, nil, &owner) == nil {
			stream.Room.Anchor = owner.Data.Info.Name
		}
		cancel()
	}
	for i := range stream.Sources {
		stream.Sources[i].Headers, err = c.mediaHeaders(Bilibili, stream.Sources[i].URL, "https://live.bilibili.com/"+id)
		if err != nil {
			return nil, err
		}
	}
	if len(route) == 0 || !stream.SelectSource(route[0]) {
		stream.SelectSource(stream.Sources[0].ID)
	}
	return stream, nil
}

func parseBiliPlay(play biliPlay) (*Stream, error) {
	type candidate struct {
		url, format string
		codec       biliCodec
		score       int
	}
	var candidates []candidate
	names := map[int]string{}
	p := play.Data.PlayURLInfo.PlayURL
	for _, d := range p.Descriptions {
		names[d.ID] = d.Name
	}
	for _, s := range p.Streams {
		for _, f := range s.Formats {
			for _, co := range f.Codecs {
				if co.Name != "avc" && co.Name != "" {
					continue
				}
				for _, u := range co.URLs {
					raw := u.Host + co.Base + u.Extra
					parsed, err := url.Parse(raw)
					if err != nil || parsed.Scheme != "https" && parsed.Scheme != "http" || parsed.Hostname() == "" {
						continue
					}
					score := 0
					if f.Name != "flv" {
						score += 2
					}
					if strings.Contains(parsed.Hostname(), "mcdn") {
						score += 4
					}
					if parsed.Scheme != "https" {
						score++
					}
					candidates = append(candidates, candidate{raw, f.Name, co, score})
				}
			}
		}
	}
	if len(candidates) == 0 {
		return nil, errors.New("B 站未返回可播放的 H.264 直播流；可在账号设置中登录后重试")
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score < candidates[j].score })
	chosen := candidates[0]
	result := &Stream{URL: chosen.url, Format: chosen.format, QualityID: chosen.codec.Current}
	seen := map[string]bool{}
	for _, item := range candidates {
		if item.codec.Current != chosen.codec.Current {
			continue
		}
		u, _ := url.Parse(item.url)
		id := u.Hostname() + "|" + item.format
		if seen[id] {
			continue
		}
		seen[id] = true
		format := "FLV"
		if item.format != "flv" {
			format = "HLS"
		}
		result.Sources = append(result.Sources, Source{ID: id, Name: fmt.Sprintf("线路 %d", len(result.Sources)+1), Format: format, URL: item.url})
	}
	result.SourceID = result.Sources[0].ID
	for _, id := range chosen.codec.Accepted {
		name := names[id]
		if name == "" {
			name = strconv.Itoa(id)
		}
		result.Qualities = append(result.Qualities, Quality{id, name})
	}
	return result, nil
}
