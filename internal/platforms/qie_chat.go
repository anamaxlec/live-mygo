package platforms

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

func qiePacket(op uint32, body []byte) []byte {
	data := make([]byte, 16+len(body))
	binary.BigEndian.PutUint32(data, uint32(len(data)))
	binary.BigEndian.PutUint16(data[4:], 16)
	binary.BigEndian.PutUint16(data[6:], 1)
	binary.BigEndian.PutUint32(data[8:], op)
	binary.BigEndian.PutUint32(data[12:], 1)
	copy(data[16:], body)
	return data
}
func qieFrames(data []byte, depth int, visit func(uint32, []byte)) {
	if depth > 8 {
		return
	}
	for len(data) >= 16 {
		n, header := int(binary.BigEndian.Uint32(data)), int(binary.BigEndian.Uint16(data[4:]))
		if n < 16 || n > len(data) || header < 16 || header > n {
			return
		}
		op := binary.BigEndian.Uint32(data[8:])
		payload := data[header:n]
		if op == 9 {
			qieFrames(payload, depth+1, visit)
		} else {
			visit(op, payload)
		}
		data = data[n:]
	}
}
func (c *Client) qieChat(ctx context.Context, id string, message func(ChatMessage), status func(string)) error {
	data, err := c.page(ctx, "https://p7api.qiecdn.com/api/v1/comet/list?client_type=pc_web", "https://www.qie.tv/", UserAgent, "")
	if err != nil {
		return err
	}
	var nodes struct {
		Nodes []string `json:"nodes"`
	}
	if json.Unmarshal(data, &nodes) != nil || len(nodes.Nodes) == 0 {
		return errors.New("企鹅体育未返回弹幕节点")
	}
	var conn *websocket.Conn
	for _, node := range nodes.Nodes {
		// 节点由官网提供，仅连接官方域名下的加密端点。
		if !strings.HasSuffix(node, ".qiecdn.com") || strings.ContainsAny(node, "/:@") {
			continue
		}
		handshake, cancel := context.WithTimeout(ctx, 4*time.Second)
		conn, _, err = websocket.Dial(handshake, "wss://"+node+"/sub", &websocket.DialOptions{HTTPClient: c.HTTP, HTTPHeader: http.Header{"User-Agent": {UserAgent}, "Origin": {"https://www.qie.tv"}}})
		cancel()
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	if conn == nil {
		if err == nil {
			err = errors.New("企鹅体育未提供可连接的弹幕节点")
		}
		return &ChatConnectionError{Cause: err}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(2 << 20)
	device := make([]byte, 16)
	if _, err = rand.Read(device); err != nil {
		return err
	}
	auth, _ := json.Marshal(map[string]any{"uid": 0, "token": "", "roomId": "online://" + id, "deviceId": hex.EncodeToString(device), "platform": "pc_web", "unAccepts": []int{}})
	if err = conn.Write(ctx, websocket.MessageBinary, qiePacket(7, auth)); err != nil {
		return err
	}
	session, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	defer func() { cancel(); wg.Wait() }()
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-session.Done():
				return
			case <-ticker.C:
				if conn.Write(session, websocket.MessageBinary, qiePacket(2, nil)) != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()
	var count uint64
	for {
		_, data, e := conn.Read(session)
		if e != nil {
			return e
		}
		var protocolErr error
		qieFrames(data, 0, func(op uint32, body []byte) {
			if op == 8 {
				var auth struct {
					Status int `json:"status"`
				}
				if json.Unmarshal(body, &auth) != nil || auth.Status != 0 {
					protocolErr = errors.New("企鹅体育弹幕会话未通过认证")
					return
				}
				status("已连接")
				protocolErr = conn.Write(session, websocket.MessageBinary, qiePacket(2, nil))
				return
			}
			if op != 2000 {
				return
			}
			var msg struct {
				From struct {
					ID   json.Number `json:"u"`
					Nick string      `json:"n"`
				} `json:"from"`
				Message struct {
					Content string `json:"content"`
				} `json:"msg"`
			}
			if json.Unmarshal(body, &msg) != nil || msg.Message.Content == "" {
				return
			}
			count++
			message(ChatMessage{ID: count, UserID: string(msg.From.ID), User: msg.From.Nick, Text: msg.Message.Content, Color: 0xffffff, Received: time.Now()})
		})
		if protocolErr != nil {
			return protocolErr
		}
	}
}
