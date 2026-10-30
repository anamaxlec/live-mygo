package platforms

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/rand"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/dop251/goja"
)

//go:embed scripts/douyin-webmssdk.js
var douyinSDK string
var douyinScript struct {
	sync.Once
	program *goja.Program
	err     error
}

func douyinSignature(ctx context.Context, room, user string) (string, error) {
	douyinScript.Do(func() { douyinScript.program, douyinScript.err = goja.Compile("douyin-webmssdk.js", douyinSDK, false) })
	if douyinScript.err != nil {
		return "", douyinScript.err
	}
	rt := goja.New()
	timer := time.AfterFunc(2*time.Second, func() { rt.Interrupt("signature timeout") })
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { rt.Interrupt(ctx.Err()) })
	defer stop()
	rt.Set("console", map[string]any{"log": func(...any) {}, "error": func(...any) {}, "warn": func(...any) {}})
	if _, err := rt.RunProgram(douyinScript.program); err != nil {
		return "", err
	}
	stub := fmt.Sprintf("live_id=1,aid=6383,version_code=180800,webcast_sdk_version=1.3.0,room_id=%s,sub_room_id=,sub_channel_id=,did_rule=3,user_unique_id=%s,device_platform=web,device_type=,ac=,identity=audience", room, user)
	fn, ok := goja.AssertFunction(rt.Get("getMSSDKSignature"))
	if !ok {
		return "", errors.New("抖音弹幕签名函数不可用")
	}
	v, err := fn(goja.Undefined(), rt.ToValue(fmt.Sprintf("%x", md5.Sum([]byte(stub)))), rt.ToValue(douyinUA))
	if err != nil {
		return "", err
	}
	return v.String(), nil
}
func (c *Client) douyinChat(ctx context.Context, id string, message func(ChatMessage), status func(string)) error {
	state, cookie, err := c.douyinPage(ctx, id)
	if err != nil {
		return err
	}
	cookie = c.douyinChatCookie(ctx, id, cookie)
	room := state.RoomStore.Info.ID
	user := state.UserStore.Odin.ID
	if user == "" {
		user = state.UserStore.User.ID
	}
	if user == "" {
		v, e := rand.Int(rand.Reader, big.NewInt(900000000000))
		if e != nil {
			return e
		}
		user = strconv.FormatInt(v.Int64()+100000000000, 10)
	}
	signature, err := douyinSignature(ctx, room, user)
	if err != nil {
		return fmt.Errorf("抖音弹幕签名失败：%w", err)
	}
	q := url.Values{"app_name": {"douyin_web"}, "version_code": {"180800"}, "webcast_sdk_version": {"1.3.0"}, "update_version_code": {"1.3.0"}, "compress": {"gzip"}, "resp_content_type": {"protobuf"}, "cursor": {fmt.Sprintf("h-1_t-%d_r-1_d-1_u-1", time.Now().UnixMilli())}, "host": {"https://live.douyin.com"}, "aid": {"6383"}, "live_id": {"1"}, "did_rule": {"3"}, "debug": {"false"}, "maxCacheMessageNumber": {"20"}, "endpoint": {"live_pc"}, "support_wrds": {"1"}, "im_path": {"/webcast/im/fetch/"}, "user_unique_id": {user}, "device_platform": {"web"}, "cookie_enabled": {"true"}, "screen_width": {"1920"}, "screen_height": {"1080"}, "browser_language": {"zh-CN"}, "browser_platform": {"Win32"}, "browser_name": {"Mozilla"}, "browser_version": {strings.TrimPrefix(douyinUA, "Mozilla/")}, "browser_online": {"true"}, "tz_name": {"Asia/Shanghai"}, "identity": {"audience"}, "room_id": {room}, "heartbeatDuration": {"0"}, "signature": {signature}}
	endpoints := []string{"wss://webcast3-ws-web-lq.douyin.com/webcast/im/push/v2/", "wss://webcast5-ws-web-lf.douyin.com/webcast/im/push/v2/", "wss://webcast5-ws-web-hl.douyin.com/webcast/im/push/v2/"}
	var initial protoFields
	heartbeat := 10 * time.Second
	prefetch, finish := context.WithTimeout(ctx, 5*time.Second)
	if data, e := c.douyinSigned(prefetch, "https://live.douyin.com/webcast/im/fetch/", q, douyinUA, cookie); e == nil {
		initial, _ = douyinResponse(data)
		cursor := string(initial.first(2).Bytes)
		if cursor == "" {
			cursor = string(initial.first(11).Bytes)
		}
		if cursor != "" {
			q.Set("cursor", cursor)
			q.Set("internal_ext", string(initial.first(5).Bytes))
		}
		if interval := initial.first(8).Number; interval > 0 {
			heartbeat = time.Duration(max(1000, min(120000, interval))) * time.Millisecond
			q.Set("heartbeatDuration", strconv.FormatUint(interval, 10))
		}
		if u, e := url.Parse(string(initial.first(10).Bytes)); e == nil && (u.Scheme == "wss" || u.Scheme == "https") && strings.HasSuffix(u.Hostname(), ".douyin.com") {
			u.Scheme = "wss"
			if u.Path == "" || u.Path == "/" {
				u.Path = "/webcast/im/push/v2/"
			}
			u.RawQuery = ""
			endpoints = append([]string{u.String()}, endpoints...)
		}
	}
	finish()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	headers := http.Header{"Accept": {"application/json, text/plain, */*"}, "Accept-Language": {"zh-CN,zh;q=0.9,en;q=0.8"}, "Cache-Control": {"no-cache"}, "Pragma": {"no-cache"}, "User-Agent": {douyinUA}, "Origin": {"https://live.douyin.com"}, "Referer": {"https://live.douyin.com/" + id}, "Cookie": {cookie}}
	var conn *websocket.Conn
	for _, endpoint := range endpoints {
		handshake, cancel := context.WithTimeout(ctx, 4*time.Second)
		conn, _, err = websocket.Dial(handshake, endpoint+"?"+q.Encode(), &websocket.DialOptions{HTTPClient: c.HTTP, HTTPHeader: headers})
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if err != nil {
		return &ChatConnectionError{Cause: err}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(2 << 20)
	if err = conn.Write(ctx, websocket.MessageBinary, protoBytes(nil, 7, []byte("hb"))); err != nil {
		return err
	}
	session, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-session.Done():
				return
			case <-ticker.C:
				if conn.Write(session, websocket.MessageBinary, protoBytes(nil, 7, []byte("hb"))) != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()
	status("已连接")
	var count uint64
	emit := func(msg ChatMessage) { count++; msg.ID = count; msg.Received = time.Now(); message(msg) }
	visitDouyinMessages(initial, emit)
	for {
		_, data, e := conn.Read(session)
		if e != nil {
			return e
		}
		frame, e := readProto(data)
		if e != nil {
			continue
		}
		response, e := douyinResponse(frame.first(8).Bytes)
		if e != nil {
			continue
		}
		if response.first(9).Number != 0 {
			ack := protoNumber(nil, 2, frame.first(2).Number)
			ack = protoBytes(ack, 7, []byte("ack"))
			ack = protoBytes(ack, 8, response.first(5).Bytes)
			if e = conn.Write(session, websocket.MessageBinary, ack); e != nil {
				return e
			}
		}
		visitDouyinMessages(response, emit)
	}
}

func (c *Client) douyinChatCookie(ctx context.Context, id, base string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, "https://live.douyin.com/"+url.PathEscape(id), nil)
	if err != nil {
		return base
	}
	req.Header.Set("User-Agent", douyinUA)
	req.Header.Set("Referer", req.URL.String())
	req.Header.Set("Cookie", base)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return base
	}
	defer resp.Body.Close()
	values := map[string]string{}
	var order []string
	for _, item := range req.Cookies() {
		values[item.Name] = item.Value
		order = append(order, item.Name)
	}
	for _, item := range resp.Cookies() {
		if item.Name != "ttwid" && item.Name != "__ac_nonce" && item.Name != "msToken" {
			continue
		}
		if _, exists := values[item.Name]; exists {
			continue
		}
		values[item.Name] = item.Value
		order = append(order, item.Name)
	}
	var parts []string
	for _, name := range order {
		parts = append(parts, name+"="+values[name])
	}
	return strings.Join(parts, "; ")
}

func douyinResponse(payload []byte) (protoFields, error) {
	if len(payload) > 2 && payload[0] == 0x1f && payload[1] == 0x8b {
		z, e := gzip.NewReader(bytes.NewReader(payload))
		if e != nil {
			return nil, e
		}
		payload, e = io.ReadAll(io.LimitReader(z, (2<<20)+1))
		z.Close()
		if e != nil {
			return nil, e
		}
		if len(payload) > 2<<20 {
			return nil, errors.New("弹幕解压数据过大")
		}
	}
	return readProto(payload)
}
func visitDouyinMessages(response protoFields, emit func(ChatMessage)) {
	for _, entry := range response[1] {
		envelope, e := readProto(entry.Bytes)
		if e != nil || string(envelope.first(1).Bytes) != "WebcastChatMessage" {
			continue
		}
		chat, e := readProto(envelope.first(2).Bytes)
		if e != nil {
			continue
		}
		sender, e := readProto(chat.first(2).Bytes)
		if e != nil {
			continue
		}
		text := string(chat.first(3).Bytes)
		if text == "" {
			continue
		}
		uid := string(sender.first(1028).Bytes)
		if uid == "" {
			uid = strconv.FormatUint(sender.first(1).Number, 10)
		}
		emit(ChatMessage{UserID: uid, User: string(sender.first(3).Bytes), Text: text, Color: 0xffffff})
	}
}
