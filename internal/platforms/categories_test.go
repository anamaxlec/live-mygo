package platforms

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBilibiliCategoryUsesAreaRoomList(t *testing.T) {
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		q := req.URL.Query()
		if req.URL.Host != "api.live.bilibili.com" || req.URL.Path != "/room/v3/area/getRoomList" ||
			q.Get("parent_id") != "2" || q.Get("area_id") != "329" || q.Get("page") != "2" {
			t.Fatalf("分区翻页请求没有携带正确的分区和页码：%s", req.URL)
		}
		body := `{"code":0,"data":{"list":[{"roomid":5236391,"area_name":"无畏契约"}]}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	result, err := c.CategoryRooms(context.Background(), Bilibili, Category{ID: "329", ParentID: "2"}, 2)
	if err != nil || len(result.Rooms) != 1 || result.Rooms[0].Category != "无畏契约" || result.HasMore {
		t.Fatalf("分区末页解析失败：%+v %v", result, err)
	}
}

func TestHuyaCategorySendsIntegerGameID(t *testing.T) {
	c := NewClient(nil)
	c.HTTP.Transport = chatTestTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"data":[]}`
		switch req.URL.Host {
		case "live.cdn.huya.com":
			if req.URL.Query().Get("bussType") == "3" {
				body = `{"data":[{"gid":3203.0,"gameFullName":"和平精英"},{"gid":{"value":2336.0},"gameFullName":"王者荣耀"}]}`
			}
		case "www.huya.com":
			if id := req.URL.Query().Get("gameId"); id != "3203" {
				t.Fatalf("虎牙直播列表会忽略非整数 gameId：%q", id)
			}
			body = `{"status":200,"data":{"page":1,"totalPage":2,"datas":[{"profileRoom":"137528","gameFullName":"和平精英"}]}}`
		default:
			t.Fatalf("请求了未知接口：%s", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	groups, err := c.Categories(context.Background(), Huya)
	if err != nil || len(groups) < 3 || len(groups[2].Children) != 2 || groups[2].Children[1].ID != "2336" {
		t.Fatalf("虎牙手游分区解析失败：%+v %v", groups, err)
	}
	result, err := c.CategoryRooms(context.Background(), Huya, groups[2].Children[0], 1)
	if err != nil || len(result.Rooms) != 1 || result.Rooms[0].Category != "和平精英" || !result.HasMore {
		t.Fatalf("虎牙分区列表解析失败：%+v %v", result, err)
	}
}
