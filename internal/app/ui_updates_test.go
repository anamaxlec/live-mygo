//go:build darwin || windows

package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReleaseVersionsOrderStableAndNumberedBetas(t *testing.T) {
	for _, pair := range [][2]string{{"0.4.2", "0.4.1"}, {"0.4.1", "0.4.1-beta.10"}, {"0.4.1-beta.10", "v0.4.1-beta.3"}, {"1.0.0-beta", "1.0.0-alpha"}} {
		if compareReleaseVersions(pair[0], pair[1]) <= 0 || compareReleaseVersions(pair[1], pair[0]) >= 0 {
			t.Fatal("版本顺序错误", pair)
		}
	}
	if compareReleaseVersions("v0.4.1-beta.3", "0.4.1-beta.3\n") != 0 {
		t.Fatal("v 前缀影响版本比较")
	}
}

func TestReleaseFeedFiltersDraftsAndChannels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"tag_name":"v0.4.1-beta.3","prerelease":true},{"tag_name":"v0.3.0"},{"tag_name":"v0.4.1-beta.10","prerelease":true},{"tag_name":"v99.0.0","draft":true},{"tag_name":"not-a-version"}]`))
	}))
	defer server.Close()
	stable, err := fetchAppReleases(context.Background(), server.Client(), server.URL, false)
	if err != nil || len(stable) != 1 || stable[0].Tag != "v0.3.0" {
		t.Fatal("正式通道混入测试版或草稿", stable, err)
	}
	beta, err := fetchAppReleases(context.Background(), server.Client(), server.URL, true)
	if err != nil || len(beta) != 3 || beta[0].Tag != "v0.4.1-beta.10" {
		t.Fatal("测试通道没有正确排序", beta, err)
	}
}
