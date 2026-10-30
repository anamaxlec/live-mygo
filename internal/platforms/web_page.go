package platforms

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

func (c *Client) page(ctx context.Context, target, referer, agent, cookie string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", agent)
	req.Header.Set("Referer", referer)
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("网页请求失败（HTTP %d）", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (12<<20)+1))
	if err == nil && len(data) > 12<<20 {
		return nil, fmt.Errorf("网页数据过大")
	}
	return data, err
}

var flightScripts = regexp.MustCompile(`(?s)self\.__[a-zA-Z0-9_]+_f\.push\((\[.*?\])\)</script>`)
var nextData = regexp.MustCompile(`(?s)<script[^>]*id="__NEXT_DATA__"[^>]*>(.*?)</script>`)

// 只解码页面已序列化的数据，不执行页面脚本。
func flightValue(page []byte, key string, out any) error {
	for _, script := range flightScripts.FindAllSubmatch(page, -1) {
		var args []json.RawMessage
		if json.Unmarshal(script[1], &args) != nil || len(args) < 2 {
			continue
		}
		var text string
		if json.Unmarshal(args[1], &text) != nil {
			continue
		}
		marker := `"` + key + `":`
		if i := strings.Index(text, marker); i >= 0 {
			if json.NewDecoder(strings.NewReader(text[i+len(marker):])).Decode(out) == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("网页未提供 %s 数据", key)
}
