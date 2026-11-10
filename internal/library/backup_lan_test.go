package library

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestBackupTransferDownloadsAndStagesUpload(t *testing.T) {
	download := []byte(`{"preferences":{"volume":50}}`)
	transfer, err := startBackupTransfer(t.Context(), netip.MustParsePrefix("127.0.0.1/8"), "backup.json", download)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transfer.Close)
	client := &http.Client{Timeout: 5 * time.Second}
	check := func(req *http.Request, status int) (string, http.Header) {
		t.Helper()
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil || resp.StatusCode != status {
			t.Fatalf("%s %s: status %d, read error %v, body %q", req.Method, req.URL, resp.StatusCode, err, data)
		}
		return string(data), resp.Header
	}
	get := func(url string, status int) (string, http.Header) {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		return check(req, status)
	}
	page, pageHeaders := get(transfer.URL, http.StatusOK)
	if !strings.Contains(page, `action="upload"`) || !strings.Contains(page, `href="backup"`) ||
		pageHeaders.Get("Referrer-Policy") != "same-origin" {
		t.Fatalf("transfer page = %q, headers = %v", page, pageHeaders)
	}
	get(transfer.URL+"wrong", http.StatusNotFound)
	body, headers := get(transfer.URL+"backup", http.StatusOK)
	if body != string(download) || headers.Get("Content-Disposition") != "attachment; filename=backup.json" ||
		headers.Get("Cache-Control") != "no-store" {
		t.Fatalf("download = %q, headers = %v", body, headers)
	}
	post := func(data, origin string, status int) {
		t.Helper()
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		part, err := form.CreateFormFile("backup", "incoming.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, data); err != nil {
			t.Fatal(err)
		}
		if err := form.Close(); err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, transfer.URL+"upload", &body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", form.FormDataContentType())
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		check(req, status)
	}
	incoming := `[{"siteId":"douyu","roomId":"123","userName":"主播"}]`
	link, err := url.Parse(transfer.URL)
	if err != nil {
		t.Fatal(err)
	}
	origin := link.Scheme + "://" + link.Host
	post(incoming, "https://example.test", http.StatusForbidden)
	post("{}", origin, http.StatusBadRequest)
	post(incoming, origin, http.StatusOK)
	select {
	case data := <-transfer.Uploads:
		if string(data) != incoming {
			t.Fatalf("staged %q", data)
		}
	case <-time.After(time.Second):
		t.Fatal("valid upload was not staged")
	}
	post(incoming, "", http.StatusConflict)
	transfer.Close()
	select {
	case <-transfer.Done:
	case <-time.After(time.Second):
		t.Fatal("listener did not close")
	}
}
