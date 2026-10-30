package platforms

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

// 仅实现本客户端使用的 TARS 编码类型。网络解码由有界 reader 完成。
type tars []byte

func (b *tars) head(tag, kind byte) {
	if tag < 15 {
		*b = append(*b, tag<<4|kind)
	} else {
		*b = append(*b, 240|kind, tag)
	}
}
func (b *tars) number(tag byte, n int64) {
	switch {
	case n == 0:
		b.head(tag, 12)
	case n >= -128 && n <= 127:
		b.head(tag, 0)
		*b = append(*b, byte(n))
	case n >= -32768 && n <= 32767:
		b.head(tag, 1)
		*b = binary.BigEndian.AppendUint16(*b, uint16(n))
	case n >= -2147483648 && n <= 2147483647:
		b.head(tag, 2)
		*b = binary.BigEndian.AppendUint32(*b, uint32(n))
	default:
		b.head(tag, 3)
		*b = binary.BigEndian.AppendUint64(*b, uint64(n))
	}
}
func (b *tars) str(tag byte, s string) {
	if len(s) < 256 {
		b.head(tag, 6)
		*b = append(*b, byte(len(s)))
	} else {
		b.head(tag, 7)
		*b = binary.BigEndian.AppendUint32(*b, uint32(len(s)))
	}
	*b = append(*b, []byte(s)...)
}
func (b *tars) data(tag byte, v []byte) {
	b.head(tag, 13)
	b.head(0, 0)
	b.number(0, int64(len(v)))
	*b = append(*b, v...)
}
func (b *tars) object(tag byte, v tars) { b.head(tag, 10); *b = append(*b, v...); b.head(0, 11) }

type tarsField struct {
	tag   byte
	value any
}
type tarsReader struct{ *bytes.Reader }

func readTars(b []byte) ([]tarsField, error) { r := tarsReader{bytes.NewReader(b)}; return r.fields(0) }
func (r tarsReader) fields(depth int) ([]tarsField, error) {
	if depth > 24 {
		return nil, fmt.Errorf("TARS 嵌套过深")
	}
	var out []tarsField
	for r.Len() > 0 {
		f, end, err := r.field(depth + 1)
		if err != nil {
			return nil, err
		}
		if end {
			return out, nil
		}
		out = append(out, f)
	}
	return out, nil
}
func (r tarsReader) take(n int64) ([]byte, error) {
	if n < 0 || n > int64(r.Len()) {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, int(n))
	_, err := io.ReadFull(r, b)
	return b, err
}
func (r tarsReader) field(depth int) (tarsField, bool, error) {
	if depth > 24 {
		return tarsField{}, false, fmt.Errorf("TARS 嵌套过深")
	}
	h, e := r.ReadByte()
	if e != nil {
		return tarsField{}, false, e
	}
	tag, kind := h>>4, h&15
	if tag == 15 {
		tag, e = r.ReadByte()
		if e != nil {
			return tarsField{}, false, e
		}
	}
	f := tarsField{tag: tag}
	var n int64
	switch kind {
	case 0, 1, 2, 3, 4, 5:
		sizes := []int64{1, 2, 4, 8, 4, 8}
		b, err := r.take(sizes[kind])
		if err != nil {
			return f, false, err
		}
		switch len(b) {
		case 1:
			n = int64(int8(b[0]))
		case 2:
			n = int64(int16(binary.BigEndian.Uint16(b)))
		case 4:
			n = int64(int32(binary.BigEndian.Uint32(b)))
		case 8:
			n = int64(binary.BigEndian.Uint64(b))
		}
		f.value = n
	case 6, 7:
		if kind == 6 {
			v, err := r.ReadByte()
			if err != nil {
				return f, false, err
			}
			n = int64(v)
		} else {
			b, err := r.take(4)
			if err != nil {
				return f, false, err
			}
			n = int64(binary.BigEndian.Uint32(b))
		}
		b, err := r.take(n)
		if err != nil {
			return f, false, err
		}
		f.value = string(b)
	case 8, 9:
		count, _, err := r.field(depth + 1)
		if err != nil {
			return f, false, err
		}
		n, ok := count.value.(int64)
		if !ok || n < 0 || n > int64(r.Len()) {
			return f, false, fmt.Errorf("TARS 集合长度无效")
		}
		if kind == 8 {
			n *= 2
		}
		list := []tarsField{}
		for i := int64(0); i < n; i++ {
			v, _, err := r.field(depth + 1)
			if err != nil {
				return f, false, err
			}
			list = append(list, v)
		}
		f.value = list
	case 10:
		v, err := r.fields(depth + 1)
		if err != nil {
			return f, false, err
		}
		f.value = v
	case 11:
		return f, true, nil
	case 12:
		f.value = int64(0)
	case 13:
		h, err := r.ReadByte()
		if err != nil || h != 0 {
			return f, false, fmt.Errorf("TARS 字节数组格式无效")
		}
		count, _, err := r.field(depth + 1)
		if err != nil {
			return f, false, err
		}
		n, ok := count.value.(int64)
		if !ok {
			return f, false, fmt.Errorf("TARS 字节数组长度无效")
		}
		f.value, err = r.take(n)
		if err != nil {
			return f, false, err
		}
	default:
		return f, false, fmt.Errorf("TARS 类型无效：%d", kind)
	}
	return f, false, nil
}
func tarsValue(fields []tarsField, tag byte) any {
	for _, f := range fields {
		if f.tag == tag {
			return f.value
		}
	}
	return nil
}
func (c *Client) huyaToken(ctx context.Context, stream string) (string, error) {
	var user, request, wrapped, params, packet tars
	user.number(0, 0)
	user.str(1, "")
	user.str(2, "")
	user.str(3, "pc_exe&7060000&official")
	user.str(4, "")
	user.number(5, 0)
	request.str(0, "")
	request.str(1, stream)
	request.number(2, 0)
	request.object(3, user)
	request.number(4, 66)
	wrapped.object(0, request)
	params.head(0, 8)
	params.number(0, 1)
	params.str(0, "tReq")
	params.data(1, wrapped)
	packet.number(1, 3)
	packet.number(2, 0)
	packet.number(3, 0)
	packet.number(4, 1)
	packet.str(5, "liveui")
	packet.str(6, "getCdnTokenInfoEx")
	packet.data(7, params)
	packet.number(8, 0)
	packet.head(9, 8)
	packet.number(0, 0)
	packet.head(10, 8)
	packet.number(0, 0)
	body := binary.BigEndian.AppendUint32(nil, uint32(len(packet)+4))
	body = append(body, packet...)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://wup.huya.com", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", huyaUA)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Referer", "https://m.huya.com/")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("虎牙线路凭据请求失败（%d）", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	// 凭据是响应中唯一含 wsSecret 的 TARS 字符串，按字段解析，而非读入平台脚本。
	if len(b) < 4 {
		return "", io.ErrUnexpectedEOF
	}
	fields, err := readTars(b[4:])
	if err != nil {
		return "", err
	}
	var find func(any) string
	find = func(v any) string {
		switch v := v.(type) {
		case string:
			if regexp.MustCompile(`^wsSecret=[0-9a-f]+&`).MatchString(v) {
				return v
			}
		case []byte:
			f, e := readTars(v)
			if e == nil {
				return find(f)
			}
		case []tarsField:
			for _, f := range v {
				if result := find(f.value); result != "" {
					return result
				}
			}
		}
		return ""
	}
	if token := find(fields); token != "" {
		return token, nil
	}
	return "", fmt.Errorf("虎牙未返回线路凭据")
}
