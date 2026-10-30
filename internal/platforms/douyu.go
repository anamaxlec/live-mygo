package platforms

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func (c *Client) douyu(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	ref := "https://www.douyu.com/" + id
	var detail struct {
		Room struct {
			ID     json.Number `json:"room_id"`
			Title  string      `json:"room_name"`
			Anchor string      `json:"nickname"`
			Owner  string      `json:"owner_name"`
			Cover  string      `json:"room_pic"`
			Status int         `json:"show_status"`
			Loop   int         `json:"videoLoop"`
		} `json:"room"`
	}
	if err := c.request(ctx, Douyu, http.MethodGet, "https://www.douyu.com/betard/"+id, ref, nil, &detail); err != nil {
		if strings.Contains(err.Error(), "无法解析的数据") {
			return nil, errors.New("斗鱼直播间当前未开播、已关闭或被平台限制")
		}
		return nil, err
	}
	if detail.Room.ID == "" {
		return nil, errors.New("未找到斗鱼直播间")
	}
	id = string(detail.Room.ID)
	if detail.Room.Status != 1 || detail.Room.Loop == 1 {
		return nil, errors.New("该斗鱼房间当前未直播")
	}
	var script struct {
		Error int               `json:"error"`
		Data  map[string]string `json:"data"`
	}
	if err := c.request(ctx, Douyu, http.MethodGet, "https://www.douyu.com/swf_api/homeH5Enc?rids="+id, ref, nil, &script); err != nil {
		return nil, err
	}
	js := script.Data["room"+id]
	if script.Error != 0 || js == "" {
		return nil, errors.New("斗鱼未返回取流签名脚本")
	}
	didBytes := make([]byte, 16)
	if _, err := rand.Read(didBytes); err != nil {
		return nil, err
	}
	did := hex.EncodeToString(didBytes)
	args, err := signDouyu(ctx, js, id, did, time.Now().Unix())
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(args)
	if err != nil {
		return nil, errors.New("斗鱼签名结果格式异常")
	}
	cdn := ""
	if len(route) > 0 {
		cdn = route[0]
	}
	// 只带签名和 rate。iar/ive 等自动画质参数会让服务端忽略所选清晰度，
	// 表现为列表里怎么点都停在 540p。
	form.Set("rate", strconv.Itoa(quality))
	form.Set("ver", "Douyu_224011102")
	if cdn != "" {
		form.Set("cdn", cdn)
	}
	var play struct {
		Error int `json:"error"`
		Data  struct {
			CDN  string `json:"cdn"`
			CDNs []struct {
				ID   string `json:"cdn"`
				Name string `json:"name"`
			} `json:"cdnsWithName"`
			Base      string `json:"rtmp_url"`
			Live      string `json:"rtmp_live"`
			Rate      int    `json:"rate"`
			Qualities []struct {
				ID   int    `json:"rate"`
				Name string `json:"name"`
			} `json:"multirates"`
		} `json:"data"`
	}
	if err = c.request(ctx, Douyu, http.MethodPost, "https://www.douyu.com/lapi/live/getH5Play/"+id, ref, form, &play); err != nil {
		return nil, err
	}
	if play.Error != 0 {
		return nil, fmt.Errorf("斗鱼取流失败（code %d），可登录后重试", play.Error)
	}
	if play.Data.Base == "" || play.Data.Live == "" {
		return nil, errors.New("斗鱼未返回播放地址")
	}
	raw := strings.TrimRight(play.Data.Base, "/") + "/" + html.UnescapeString(play.Data.Live)
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("斗鱼返回了无效的直播流地址")
	}
	anchor := detail.Room.Anchor
	if anchor == "" {
		anchor = detail.Room.Owner
	}
	s := &Stream{Room: Room{Platform: Douyu, ID: id, Title: detail.Room.Title, Anchor: anchor, Live: true, Cover: detail.Room.Cover}, URL: raw, QualityID: play.Data.Rate, Format: "http-flv"}
	for _, q := range play.Data.Qualities {
		s.Qualities = append(s.Qualities, Quality{q.ID, q.Name})
	}
	s.Headers, err = c.mediaHeaders(Douyu, raw, "https://www.douyu.com/"+id)
	if err != nil {
		return nil, err
	}
	s.SourceID = play.Data.CDN
	if s.SourceID == "" {
		s.SourceID = cdn
	}
	for _, cdn := range play.Data.CDNs {
		name := cdn.Name
		if name == "" {
			name = fmt.Sprintf("线路 %d", len(s.Sources)+1)
		}
		route := Source{ID: cdn.ID, Name: name, Format: "FLV"}
		if cdn.ID == s.SourceID {
			route.URL, route.Headers = raw, s.Headers
		}
		s.Sources = append(s.Sources, route)
	}
	if s.SourceID == "" {
		s.Sources = append([]Source{{ID: "", Name: "自动线路", Format: "FLV", URL: raw, Headers: s.Headers}}, s.Sources...)
	}
	if len(s.Sources) == 0 {
		s.Sources = []Source{{ID: s.SourceID, Name: "默认线路", Format: "FLV", URL: raw, Headers: s.Headers}}
	}
	return s, err
}

// 平台返回的脚本仅运行在独立 Goja VM 中；不提供文件、网络或宿主账号接口。
func signDouyu(ctx context.Context, script, rid, did string, ts int64) (string, error) {
	vm := goja.New()
	crypto := vm.NewObject()
	if err := crypto.Set("MD5", func(text string) string { sum := md5.Sum([]byte(text)); return hex.EncodeToString(sum[:]) }); err != nil {
		return "", err
	}
	if err := vm.Set("CryptoJS", crypto); err != nil {
		return "", err
	}
	deadline, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-deadline.Done():
			vm.Interrupt("签名超时或取消")
		case <-done:
		}
	}()
	if _, err := vm.RunString(script); err != nil {
		return "", errors.New("斗鱼签名脚本执行失败")
	}
	fn, ok := goja.AssertFunction(vm.Get("ub98484234"))
	if !ok {
		return "", errors.New("斗鱼签名函数缺失")
	}
	value, err := fn(goja.Undefined(), vm.ToValue(rid), vm.ToValue(did), vm.ToValue(strconv.FormatInt(ts, 10)))
	if err != nil {
		return "", errors.New("斗鱼签名失败")
	}
	return value.String(), nil
}
