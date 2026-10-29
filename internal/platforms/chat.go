package platforms

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/coder/websocket"
)

type ChatMessage struct {
	ID         uint64
	UserID     string
	User       string
	Text       string
	Color      uint32
	Kind       string
	Price      float64
	Medal      string
	MedalLevel int
	Emotes     []ChatEmote
	Received   time.Time
}

type ChatEmote struct{ Text, URL string }

type ChatConnectionError struct{ Cause error }

func (e *ChatConnectionError) Error() string {
	if strings.Contains(strings.ToLower(e.Cause.Error()), "tls") {
		return "弹幕加密连接失败，请检查网络或稍后重试"
	}
	return "弹幕连接失败，请稍后重试"
}
func (e *ChatConnectionError) Unwrap() error { return e.Cause }

// ListenChat 只订阅公开弹幕，不发送聊天消息；调用方取消 context 即关闭连接。
func (c *Client) ListenChat(ctx context.Context, p Platform, id string, message func(ChatMessage), status func(string)) error {
	var err error
	p, id, err = ParseRoom(p, id)
	if err != nil {
		return err
	}
	target := "wss://danmuproxy.douyu.com:8506/"
	headers := http.Header{"User-Agent": {UserAgent}, "Origin": {"https://www.douyu.com"}}
	var initial [][]byte
	heartbeat := douyuPacket("type@=mrkl/")
	if p == Bilibili {
		session, err := c.biliGuest(ctx)
		if err != nil {
			return err
		}
		q := url.Values{"id": {id}, "type": {"0"}}
		if err = session.wbi(ctx, q); err != nil {
			return err
		}
		var info struct {
			Code int `json:"code"`
			Data struct {
				Token string `json:"token"`
				Hosts []struct {
					Host string `json:"host"`
					Port int    `json:"wss_port"`
				} `json:"host_list"`
			} `json:"data"`
		}
		if err = session.request(ctx, p, http.MethodGet, "https://api.live.bilibili.com/xlive/web-room/v1/index/getDanmuInfo?"+q.Encode(), "https://live.bilibili.com/", nil, &info); err != nil {
			return err
		}
		if info.Code != 0 || info.Data.Token == "" {
			return errors.New("平台暂未提供弹幕会话，可登录后重新连接")
		}
		host := "broadcastlv.chat.bilibili.com"
		if len(info.Data.Hosts) > 0 {
			host = info.Data.Hosts[0].Host
			if !strings.HasSuffix(host, ".bilibili.com") || strings.ContainsAny(host, "/:@") {
				return errors.New("弹幕服务器地址无效")
			}
			if port := info.Data.Hosts[0].Port; port > 0 && port != 443 && port < 65536 {
				host += ":" + strconv.Itoa(port)
			}
		}
		target = "wss://" + host + "/sub"
		headers.Set("Origin", "https://live.bilibili.com")
		cookie := session.guestBili
		if c.Cookies != nil {
			saved, err := c.Cookies.Header(p, "https://"+host+"/sub")
			if err != nil {
				return err
			}
			if saved != "" {
				cookie = saved + "; " + cookie
			}
		}
		if cookie != "" {
			headers.Set("Cookie", cookie)
		}
		var buvid string
		var uid int64
		request := http.Request{Header: http.Header{"Cookie": {cookie}}}
		for _, v := range request.Cookies() {
			if v.Name == "buvid3" {
				buvid = v.Value
			}
			if v.Name == "DedeUserID" {
				uid, _ = strconv.ParseInt(v.Value, 10, 64)
			}
		}
		room, _ := strconv.ParseInt(id, 10, 64)
		var queue [4]byte
		if _, err = rand.Read(queue[:]); err != nil {
			return err
		}
		auth, _ := json.Marshal(map[string]any{"uid": uid, "roomid": room, "protover": 2, "buvid": buvid, "platform": "web", "type": 2, "key": info.Data.Token, "support_ack": true, "queue_uuid": hex.EncodeToString(queue[:]), "scene": "room"})
		initial = [][]byte{biliPacket(auth, 7)}
		heartbeat = biliPacket(nil, 2)
	} else {
		initial = [][]byte{douyuPacket("type@=loginreq/roomid@=" + id + "/"), douyuPacket("type@=joingroup/rid@=" + id + "/gid@=-9999/")}
	}
	status("正在连接")
	conn, err := c.dialChat(ctx, p, target, headers)
	if err != nil {
		return &ChatConnectionError{Cause: err}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(4 << 20)
	for _, packet := range initial {
		if err = conn.Write(ctx, websocket.MessageBinary, packet); err != nil {
			return errors.New("弹幕订阅失败")
		}
	}
	heartCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		interval := 30 * time.Second
		if p == Douyu {
			interval = 45 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartCtx.Done():
				return
			case <-ticker.C:
				if conn.Write(heartCtx, websocket.MessageBinary, heartbeat) != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()
	ready := false
	accept := func() {
		if !ready {
			ready = true
			_ = conn.Write(ctx, websocket.MessageBinary, heartbeat)
			status("已连接")
		}
	}
	var pending []byte
	for {
		// 两个平台均有 30 秒心跳；连接半断开时不要永久停在“已连接”。
		readCtx, readCancel := context.WithTimeout(ctx, 75*time.Second)
		_, data, err := conn.Read(readCtx)
		readCancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("弹幕连接已中断")
		}
		if len(pending)+len(data) > 4<<20 {
			return errors.New("弹幕数据过大")
		}
		pending = append(pending, data...)
		if p == Bilibili {
			pending, err = decodeBili(pending, 0, message, accept, func(ack []byte) { _ = conn.Write(ctx, websocket.MessageBinary, biliPacket(ack, 24)) })
		} else {
			pending, err = decodeDouyu(pending, message, accept, id)
		}
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			pending = nil
		}
	}
}

// 对齐 Simple Live 的斗鱼候选列表与三秒握手超时，保持系统 TLS 验证。
func (c *Client) dialChat(ctx context.Context, p Platform, target string, headers http.Header) (*websocket.Conn, error) {
	client := c.HTTP
	if p == Douyu {
		client = c.douyuChatClient()
		if client != c.HTTP {
			defer client.CloseIdleConnections()
		}
	}
	targets := []string{target}
	timeout := 15 * time.Second
	if p == Douyu {
		targets = nil
		timeout = 3 * time.Second
		for port := 8501; port <= 8506; port++ {
			targets = append(targets, fmt.Sprintf("wss://danmuproxy.douyu.com:%d/", port))
		}
	}
	var err error
	for _, endpoint := range targets {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		dialCtx, cancel := context.WithTimeout(ctx, timeout)
		var conn *websocket.Conn
		conn, _, err = websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{HTTPClient: client, HTTPHeader: headers})
		cancel()
		if err == nil {
			return conn, nil
		}
	}
	return nil, err
}

func biliPacket(body []byte, op uint32) []byte {
	b := make([]byte, len(body)+16)
	binary.BigEndian.PutUint32(b, uint32(len(b)))
	binary.BigEndian.PutUint16(b[4:], 16)
	binary.BigEndian.PutUint16(b[6:], 0)
	binary.BigEndian.PutUint32(b[8:], op)
	binary.BigEndian.PutUint32(b[12:], 1)
	copy(b[16:], body)
	return b
}

func decodeBili(data []byte, depth int, emit func(ChatMessage), ready func(), acknowledge ...func([]byte)) ([]byte, error) {
	if depth > 2 {
		return nil, errors.New("弹幕压缩层级无效")
	}
	for len(data) >= 16 {
		size := int(binary.BigEndian.Uint32(data))
		header := int(binary.BigEndian.Uint16(data[4:]))
		version := binary.BigEndian.Uint16(data[6:])
		op := binary.BigEndian.Uint32(data[8:])
		if size < 16 || size > 4<<20 || header < 16 || header > size {
			return nil, errors.New("弹幕数据包无效")
		}
		if len(data) < size {
			return data, nil
		}
		body := data[header:size]
		if os.Getenv("LIVE_MYGO_CHAT_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "bili packet op=%d version=%d bytes=%d\n", op, version, len(body))
		}
		if op == 8 {
			var ack struct {
				Code int `json:"code"`
			}
			if json.Unmarshal(body, &ack) != nil || ack.Code != 0 {
				return nil, errors.New("弹幕认证失败，请重新登录后重试")
			}
			ready()
		}
		if op == 5 {
			if version == 2 {
				r, err := zlib.NewReader(bytes.NewReader(body))
				if err != nil {
					return nil, errors.New("弹幕数据无法解压")
				}
				plain, err := io.ReadAll(io.LimitReader(r, 4<<20+1))
				r.Close()
				if err != nil || len(plain) > 4<<20 {
					return nil, errors.New("弹幕解压数据过大")
				}
				if _, err = decodeBili(plain, depth+1, emit, ready, acknowledge...); err != nil {
					return nil, err
				}
			} else if version == 0 || version == 1 {
				parseBiliChat(body, emit, acknowledge...)
			} else {
				return nil, errors.New("平台弹幕压缩协议已更新")
			}
		}
		data = data[size:]
	}
	return data, nil
}

func parseBiliChat(data []byte, emit func(ChatMessage), acknowledge ...func([]byte)) {
	var v struct {
		Command     string            `json:"cmd"`
		Info        []json.RawMessage `json:"info"`
		Ack         bool              `json:"p_is_ack"`
		MessageID   string            `json:"msg_id"`
		MessageType json.Number       `json:"p_msg_type"`
		Data        json.RawMessage   `json:"data"`
	}
	if json.Unmarshal(data, &v) != nil {
		return
	}
	if os.Getenv("LIVE_MYGO_CHAT_DEBUG") == "1" {
		fmt.Fprintln(os.Stderr, "bili command", v.Command)
	}
	if v.Ack && v.MessageID != "" && v.Command != "" && len(acknowledge) > 0 {
		if kind, err := v.MessageType.Int64(); err == nil {
			payload, _ := json.Marshal(map[string]any{"msg_id": v.MessageID, "cmd": v.Command, "p_msg_type": kind})
			acknowledge[0](payload)
		}
	}
	if v.Command == "SUPER_CHAT_MESSAGE" {
		var sc struct {
			UID   json.Number `json:"uid"`
			Text  string      `json:"message"`
			Price float64     `json:"price"`
			User  struct {
				Name string `json:"uname"`
			} `json:"user_info"`
		}
		if json.Unmarshal(v.Data, &sc) == nil && cleanChat(sc.Text, 300) != "" {
			emit(ChatMessage{UserID: string(sc.UID), User: cleanChat(sc.User.Name, 60), Text: cleanChat(sc.Text, 300), Kind: "superchat", Price: sc.Price, Color: 0xffffff})
		}
		return
	}
	if !strings.HasPrefix(v.Command, "DANMU_MSG") || len(v.Info) < 3 {
		return
	}
	var text, user string
	var who, detail []json.RawMessage
	var color uint32
	if json.Unmarshal(v.Info[1], &text) != nil || json.Unmarshal(v.Info[2], &who) != nil || len(who) < 2 {
		return
	}
	_ = json.Unmarshal(who[1], &user)
	if json.Unmarshal(v.Info[0], &detail) == nil && len(detail) > 3 {
		_ = json.Unmarshal(detail[3], &color)
	}
	if text = cleanChat(text, 300); text != "" {
		msg := ChatMessage{User: cleanChat(user, 60), Text: text, Color: color}
		var uid json.Number
		if len(who) > 0 && json.Unmarshal(who[0], &uid) == nil {
			msg.UserID = string(uid)
		}
		if len(v.Info) > 3 {
			var medal []json.RawMessage
			if json.Unmarshal(v.Info[3], &medal) == nil && len(medal) > 1 {
				_ = json.Unmarshal(medal[0], &msg.MedalLevel)
				_ = json.Unmarshal(medal[1], &msg.Medal)
				msg.Medal = cleanChat(msg.Medal, 16)
			}
		}
		if len(detail) > 13 {
			var single struct {
				URL string `json:"url"`
			}
			if json.Unmarshal(detail[13], &single) == nil && safeEmoteURL(single.URL) != "" {
				msg.Emotes = append(msg.Emotes, ChatEmote{Text: text, URL: safeEmoteURL(single.URL)})
			}
		}
		if len(detail) > 15 {
			var extra struct {
				Extra string `json:"extra"`
			}
			var content struct {
				Emotes map[string]struct {
					URL string `json:"url"`
				} `json:"emots"`
			}
			if json.Unmarshal(detail[15], &extra) == nil && json.Unmarshal([]byte(extra.Extra), &content) == nil {
				for token, item := range content.Emotes {
					if token != "" && len(msg.Emotes) < 16 && strings.Contains(text, token) && safeEmoteURL(item.URL) != "" {
						msg.Emotes = append(msg.Emotes, ChatEmote{Text: token, URL: safeEmoteURL(item.URL)})
					}
				}
			}
		}
		emit(msg)
	}
}

func safeEmoteURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, domain := range []string{"hdslb.com", "biliimg.com"} {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return u.String()
		}
	}
	return ""
}

func douyuPacket(body string) []byte {
	b := make([]byte, len([]byte(body))+13)
	binary.LittleEndian.PutUint32(b, uint32(len(b)-4))
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-4))
	binary.LittleEndian.PutUint16(b[8:], 689)
	copy(b[12:], body)
	return b
}

func decodeDouyu(data []byte, emit func(ChatMessage), ready func(), room ...string) ([]byte, error) {
	for len(data) >= 12 {
		size := int(binary.LittleEndian.Uint32(data)) + 4
		if size < 13 || size > 4<<20 || binary.LittleEndian.Uint32(data) != binary.LittleEndian.Uint32(data[4:]) {
			return nil, errors.New("斗鱼弹幕数据包无效")
		}
		if len(data) < size {
			return data, nil
		}
		fields := parseDouyuFields(string(data[12 : size-1]))
		if len(room) > 0 && fields["rid"] != "" && fields["rid"] != room[0] {
			data = data[size:]
			continue
		}

		if fields["type"] == "loginres" {
			ready()
		}
		if fields["type"] == "chatmsg" {
			color, _ := strconv.Atoi(fields["col"])
			colors := []uint32{0xffffff, 0xff4040, 0x1e87f0, 0x7ac84b, 0xff7f00, 0x9b39f4, 0xff69b4}
			if color < 0 || color >= len(colors) {
				color = 0
			}
			if text := cleanChat(fields["txt"], 300); text != "" {
				level, _ := strconv.Atoi(fields["bl"])
				emit(ChatMessage{UserID: fields["uid"], User: cleanChat(fields["nn"], 60), Text: text, Color: colors[color], Medal: cleanChat(fields["bnn"], 16), MedalLevel: level})
			}
		}
		if fields["type"] == "comm_chatmsg" {
			nested := parseDouyuFields(fields["chatmsg"])
			price, _ := strconv.ParseFloat(fields["cprice"], 64)
			emitDouyuSC(emit, nested["nn"], nested["txt"], nested["uid"], price/100)
		} else if fields["type"] == "voice_trlt" {
			nested := parseDouyuFields(strings.SplitN(fields["list"], "//", 2)[0])
			price, _ := strconv.ParseFloat(nested["realPrice"], 64)
			emitDouyuSC(emit, nested["un"], nested["content"], nested["uid"], price/100)
		}
		data = data[size:]
	}
	return data, nil
}

func parseDouyuFields(body string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Split(body, "/") {
		k, v, ok := strings.Cut(part, "@=")
		if ok {
			fields[k] = strings.NewReplacer("@S", "/", "@A", "@").Replace(v)
		}
	}
	return fields
}
func emitDouyuSC(emit func(ChatMessage), user, text, id string, price float64) {
	if text = cleanChat(text, 300); text != "" {
		emit(ChatMessage{Kind: "superchat", User: cleanChat(user, 60), UserID: id, Text: text, Price: price, Color: 0xffffff})
	}
}

func cleanChat(text string, limit int) string {
	runes := []rune(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text))
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return strings.TrimSpace(string(runes))
}
