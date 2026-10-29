package platforms

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestBiliChatCompressedAndConcatenated(t *testing.T) {
	data := []byte(`{"cmd":"DANMU_MSG","info":[[0,1,25,16777215],"你好，直播",[42,"观众"]]}`)
	plain := biliPacket(data, 5)
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	w.Write(append(plain, plain...))
	w.Close()
	packet := biliPacket(compressed.Bytes(), 5)
	binary.BigEndian.PutUint16(packet[6:], 2)
	var messages []ChatMessage
	rest, err := decodeBili(packet[:7], 0, func(v ChatMessage) { messages = append(messages, v) }, func() {})
	if err != nil || len(rest) != 7 {
		t.Fatalf("分片错误：%v", err)
	}
	rest, err = decodeBili(append(rest, packet[7:]...), 0, func(v ChatMessage) { messages = append(messages, v) }, func() {})
	if err != nil || len(rest) != 0 || len(messages) != 2 || messages[0].Text != "你好，直播" || messages[0].User != "观众" {
		t.Fatalf("压缩弹幕解析错误：%+v %v", messages, err)
	}
}

func TestDouyuChatFramingUTF8AndEscapes(t *testing.T) {
	packet := douyuPacket("type@=chatmsg/nn@=观众/txt@=你好@S直播@A朋友/col@=2/dms@=1/")
	if int(binary.LittleEndian.Uint32(packet))+4 != len(packet) {
		t.Fatal("长度应按 UTF-8 字节计算")
	}
	var messages []ChatMessage
	rest, err := decodeDouyu(append(packet, packet...), func(v ChatMessage) { messages = append(messages, v) }, func() {})
	if err != nil || len(rest) != 0 || len(messages) != 2 || messages[0].Text != "你好/直播@朋友" || messages[0].User != "观众" {
		t.Fatalf("斗鱼协议解析错误：%+v %v", messages, err)
	}
}

func TestChatRejectsInvalidLengths(t *testing.T) {
	bad := make([]byte, 16)
	binary.BigEndian.PutUint32(bad, 1)
	if _, err := decodeBili(bad, 0, func(ChatMessage) {}, func() {}); err == nil {
		t.Fatal("接受了无效 B 站数据包")
	}
	binary.LittleEndian.PutUint32(bad, 0xfffffff0)
	if _, err := decodeDouyu(bad, func(ChatMessage) {}, func() {}); err == nil {
		t.Fatal("接受了无效斗鱼数据包")
	}
}

func TestBiliChatIncludesMedalEmotesAndSuperChat(t *testing.T) {
	metadata := make([]any, 16)
	metadata[3] = 16777215
	metadata[15] = map[string]any{"extra": `{"emots":{"[笑]":{"url":"https://i0.hdslb.com/test.png"},"[坏]":{"url":"https://evil.example/test.png"}}}`}
	data, _ := json.Marshal(map[string]any{"cmd": "DANMU_MSG", "info": []any{metadata, "你好[笑][坏]", []any{42, "观众"}, []any{3, "粉丝牌"}}})
	var messages []ChatMessage
	parseBiliChat(data, func(msg ChatMessage) { messages = append(messages, msg) })
	if len(messages) != 1 || messages[0].UserID != "42" || messages[0].Medal != "粉丝牌" || messages[0].MedalLevel != 3 || len(messages[0].Emotes) != 1 {
		t.Fatalf("扩展弹幕解析错误：%+v", messages)
	}
	sc := []byte(`{"cmd":"SUPER_CHAT_MESSAGE","data":{"uid":42,"message":"支持主播","price":30,"user_info":{"uname":"观众"}}}`)
	parseBiliChat(sc, func(msg ChatMessage) { messages = append(messages, msg) })
	if len(messages) != 2 || messages[1].Kind != "superchat" || messages[1].Price != 30 || messages[1].User != "观众" {
		t.Fatalf("醒目留言解析错误：%+v", messages)
	}
}
