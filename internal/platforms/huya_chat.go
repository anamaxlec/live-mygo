package platforms

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
)

func (c *Client) huyaChat(ctx context.Context, id string, message func(ChatMessage), status func(string)) error {
	profile, err := c.huyaProfile(ctx, id)
	if err != nil {
		return err
	}
	if len(profile.Data.Stream.Lines) == 0 {
		return fmt.Errorf("直播间未开播")
	}
	line := profile.Data.Stream.Lines[0]
	conn, _, err := websocket.Dial(ctx, "wss://cdnws.api.huya.com", &websocket.DialOptions{HTTPClient: c.HTTP, HTTPHeader: http.Header{"Origin": {"https://www.huya.com"}, "User-Agent": {UserAgent}}})
	if err != nil {
		return &ChatConnectionError{Cause: err}
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	session, cancel := context.WithCancel(ctx)
	defer cancel()
	var wg sync.WaitGroup
	defer wg.Wait()
	var user, join tars
	user.number(0, profile.Data.Live.UID)
	user.number(1, 1)
	user.str(2, "")
	user.str(3, "")
	user.number(4, line.Channel)
	user.number(5, line.Sub)
	user.number(6, 0)
	user.number(7, 0)
	join.number(0, 1)
	join.data(1, user)
	if err = conn.Write(session, websocket.MessageBinary, join); err != nil {
		return err
	}
	status("已连接")
	heartbeat, _ := base64.StdEncoding.DecodeString("ABQdAAwsNgBM")
	wg.Add(1)
	go func() {
		defer wg.Done()
		timer := time.NewTicker(60 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-session.Done():
				return
			case <-timer.C:
				if conn.Write(session, websocket.MessageBinary, heartbeat) != nil {
					conn.CloseNow()
					return
				}
			}
		}
	}()
	defer cancel()
	var count uint64
	for {
		_, data, err := conn.Read(session)
		if err != nil {
			return err
		}
		msg, ok := decodeHuyaMessage(data)
		if ok {
			count++
			msg.ID = count
			msg.Received = time.Now()
			message(msg)
		}
	}
}
func decodeHuyaMessage(data []byte) (ChatMessage, bool) {
	fields, err := readTars(data)
	if err != nil {
		return ChatMessage{}, false
	}
	if tarsValue(fields, 0) != int64(7) {
		return ChatMessage{}, false
	}
	b, ok := tarsValue(fields, 1).([]byte)
	if !ok {
		return ChatMessage{}, false
	}
	push, err := readTars(b)
	if err != nil || tarsValue(push, 1) != int64(1400) {
		return ChatMessage{}, false
	}
	b, ok = tarsValue(push, 2).([]byte)
	if !ok {
		return ChatMessage{}, false
	}
	body, err := readTars(b)
	if err != nil {
		return ChatMessage{}, false
	}
	text, ok := tarsValue(body, 3).(string)
	if !ok || text == "" {
		return ChatMessage{}, false
	}
	sender, _ := tarsValue(body, 0).([]tarsField)
	name, _ := tarsValue(sender, 2).(string)
	uid, _ := tarsValue(sender, 0).(int64)
	format, _ := tarsValue(body, 6).([]tarsField)
	color, _ := tarsValue(format, 0).(int64)
	if color <= 0 {
		color = 0xffffff
	}
	return ChatMessage{UserID: strconv.FormatInt(uid, 10), User: name, Text: text, Color: uint32(color)}, true
}
