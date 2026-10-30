package platforms

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const flivePage = "https://flive.dpdns.org/f1"
const fliveChannelsURL = "https://cdn.flive.dpdns.org/channels.json"

type fliveChannel struct {
	ID       json.Number `json:"id"`
	Title    string      `json:"title"`
	URI      string      `json:"uri"`
	Status   string      `json:"status"`
	Provider string      `json:"provider"`
}

func (r fliveChannel) room() Room {
	return Room{Platform: FLive, ID: string(r.ID), Title: strings.TrimSpace(strings.TrimPrefix(r.Title, "[Embed]")), Anchor: "F1 Live", Category: "F1", Live: r.Status == "online"}
}

func (c *Client) fliveChannels(ctx context.Context) ([]fliveChannel, error) {
	data, err := c.page(ctx, fliveChannelsURL, flivePage, UserAgent, "")
	if err != nil {
		return nil, err
	}
	var out []fliveChannel
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, errors.New("频道列表暂时不可用")
	}
	return out, nil
}

func (c *Client) fliveCatalog(ctx context.Context, query string) (Catalog, error) {
	channels, err := c.fliveChannels(ctx)
	if err != nil {
		return Catalog{}, err
	}
	out := Catalog{}
	for _, ch := range channels {
		if ch.ID != "" && ch.URI != "" && strings.Contains(strings.ToLower(ch.Title), strings.ToLower(query)) {
			out.Rooms = append(out.Rooms, ch.room())
		}
	}
	return out, nil
}

func fliveFamily(title string) string {
	title = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(title, "[Embed]")))
	for _, suffix := range []string{" uhd", " fhd", " 4k", " hd", " [uk]"} {
		title = strings.TrimSuffix(title, suffix)
	}
	return strings.TrimSpace(title)
}

func (c *Client) flive(ctx context.Context, id string, quality int, route ...string) (*Stream, error) {
	channels, err := c.fliveChannels(ctx)
	if err != nil {
		return nil, err
	}
	var room, selected *fliveChannel
	for i := range channels {
		if string(channels[i].ID) == id {
			room = &channels[i]
			selected = room
			break
		}
	}
	if room == nil {
		return nil, errors.New("未找到赛事频道")
	}
	family := fliveFamily(room.Title)
	if len(route) > 0 && route[0] != "" {
		selected = nil
		for i := range channels {
			if string(channels[i].ID) == route[0] && fliveFamily(channels[i].Title) == family {
				selected = &channels[i]
				break
			}
		}
		if selected == nil {
			return nil, errors.New("该播放线路已更新，请刷新频道列表")
		}
	}
	s := &Stream{Room: room.room(), SourceID: string(selected.ID)}
	for _, ch := range channels {
		if fliveFamily(ch.Title) == family {
			s.Sources = append(s.Sources, Source{ID: string(ch.ID), Name: fmt.Sprintf("%s · 线路 %d", strings.TrimSpace(strings.TrimPrefix(ch.Title, "[Embed]")), len(s.Sources)+1), Format: "HLS"})
		}
	}
	target, referer, playlist, err := c.flivePlaylist(ctx, selected.URI, flivePage, 0)
	if err != nil {
		return s, err
	}
	variants := fliveVariants(target, playlist)
	s.Qualities = []Quality{{ID: 0, Name: "自动"}}
	s.Room.Live = true
	for i, v := range variants {
		s.Qualities = append(s.Qualities, Quality{ID: i + 1, Name: v.name})
	}
	if quality > 0 && quality <= len(variants) {
		s.QualityID = quality
		target = variants[quality-1].url
	}
	headers := map[string]string{"User-Agent": UserAgent, "Referer": referer}
	if u, e := url.Parse(referer); e == nil {
		headers["Origin"] = u.Scheme + "://" + u.Host
	}
	for i := range s.Sources {
		s.Sources[i].Headers = headers
		if s.Sources[i].ID == string(selected.ID) {
			s.Sources[i].URL = target
		}
	}
	if !s.SelectSource(string(selected.ID)) {
		return nil, errors.New("播放线路暂时不可用")
	}
	return s, nil
}

var fliveMediaURL = regexp.MustCompile(`https?://[^\s"'<>\\]+?\.m3u8[^\s"'<>\\]*`)
var fliveIframe = regexp.MustCompile(`(?i)<iframe[^>]+src=["']([^"']+)["']`)

func publicStreamURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Scheme != "https" && u.Scheme != "http" || u.Hostname() == "" {
		return nil, errors.New("频道播放地址无效")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return nil, errors.New("频道播放地址无效")
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()) {
		return nil, errors.New("频道播放地址无效")
	}
	return u, nil
}

func (c *Client) flivePlaylist(ctx context.Context, target, referer string, depth int) (string, string, []byte, error) {
	if depth > 2 {
		return "", "", nil, errors.New("该频道暂未提供可直接播放的信号")
	}
	u, err := publicStreamURL(target)
	if err != nil {
		return "", "", nil, err
	}
	publicHTTP := *c.HTTP
	publicHTTP.Jar = nil
	redirect := c.HTTP.CheckRedirect
	publicHTTP.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if _, err := publicStreamURL(req.URL.String()); err != nil {
			return err
		}
		if redirect != nil {
			return redirect(req, via)
		}
		if len(via) >= 5 {
			return errors.New("频道重定向次数过多")
		}
		return nil
	}
	public := Client{HTTP: &publicHTTP}
	data, err := public.page(ctx, u.String(), referer, UserAgent, "")
	if err != nil {
		var transport *url.Error
		if errors.As(err, &transport) {
			return "", "", nil, errors.New("频道信号连接失败，请更换线路")
		}
		return "", "", nil, fmt.Errorf("频道信号暂时不可用：%w", err)
	}
	text := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if strings.HasPrefix(text, "#EXTM3U") {
		return u.String(), referer, data, nil
	}
	text = strings.NewReplacer(`\/`, `/`, `\u0026`, `&`, "&amp;", "&").Replace(text)
	if media := fliveMediaURL.FindString(text); media != "" {
		return c.flivePlaylist(ctx, html.UnescapeString(media), u.String(), depth+1)
	}
	if iframe := fliveIframe.FindStringSubmatch(text); len(iframe) > 1 {
		next, e := url.Parse(html.UnescapeString(iframe[1]))
		if e == nil {
			return c.flivePlaylist(ctx, u.ResolveReference(next).String(), u.String(), depth+1)
		}
	}
	return "", "", nil, errors.New("该频道暂未提供可直接播放的信号")
}

type fliveVariant struct{ name, url string }

var fliveResolution = regexp.MustCompile(`(?:^|,)RESOLUTION=(\d+)x(\d+)`)
var fliveBandwidth = regexp.MustCompile(`(?:^|,)BANDWIDTH=(\d+)`)

func fliveVariants(base string, data []byte) []fliveVariant {
	u, err := url.Parse(base)
	if err != nil {
		return nil
	}
	var out []fliveVariant
	name := ""
	scan := bufio.NewScanner(strings.NewReader(string(data)))
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			name = "清晰度 " + strconv.Itoa(len(out)+1)
			attrs := strings.TrimPrefix(line, "#EXT-X-STREAM-INF:")
			if match := fliveResolution.FindStringSubmatch(attrs); len(match) > 2 {
				name = match[2] + "p"
			} else if match := fliveBandwidth.FindStringSubmatch(attrs); len(match) > 1 {
				rate, _ := strconv.ParseFloat(match[1], 64)
				name = fmt.Sprintf("%.1f Mbps", rate/1e6)
			}
		} else if name != "" && line != "" && !strings.HasPrefix(line, "#") {
			if next, e := url.Parse(line); e == nil {
				target := u.ResolveReference(next).String()
				if _, e = publicStreamURL(target); e == nil {
					out = append(out, fliveVariant{name, target})
				}
			}
			name = ""
		}
	}
	return out
}
