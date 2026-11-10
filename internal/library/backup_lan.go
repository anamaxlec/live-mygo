package library

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sync/atomic"
	"time"
)

const BackupMaxBytes = 8 << 20

const transferPage = `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Live MyGo 局域网互传</title><style>body{font:16px system-ui;margin:2rem auto;max-width:34rem;padding:0 1rem}form{margin-top:2rem}input,button{display:block;margin:1rem 0}</style><h1>Live MyGo 局域网互传</h1><p><a href="backup">下载备份</a></p><form action="upload" method="post" enctype="multipart/form-data"><label>上传备份<input type="file" name="backup" accept=".json,.txt" required></label><button>发送到 Live MyGo</button></form></html>`

type BackupTransfer struct {
	URL     string
	Uploads <-chan []byte
	Done    <-chan struct{}
	stop    context.CancelFunc
}

func (t *BackupTransfer) Close() { t.stop() }

func backupLANNetworks() []netip.Prefix {
	var networks []netip.Prefix
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			prefix, err := netip.ParsePrefix(address.String())
			if err == nil && prefix.Addr().Is4() && prefix.Addr().IsPrivate() {
				networks = append(networks, prefix)
			}
		}
	}
	return networks
}

func BackupLANAddresses() []string {
	var addresses []string
	for _, p := range backupLANNetworks() {
		addresses = append(addresses, p.Addr().String())
	}
	slices.Sort(addresses)
	return slices.Compact(addresses)
}

func StartBackupTransfer(ctx context.Context, address, filename string, data []byte) (*BackupTransfer, error) {
	for _, prefix := range backupLANNetworks() {
		if address == prefix.Addr().String() {
			return startBackupTransfer(ctx, prefix, filename, data)
		}
	}
	return nil, errors.New("请先连接局域网并选择本机地址")
}

func startBackupTransfer(parent context.Context, subnet netip.Prefix, filename string, data []byte) (*BackupTransfer, error) {
	if len(data) > BackupMaxBytes {
		return nil, errors.New("备份超过 8 MB，无法通过局域网分享")
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(subnet.Addr().String(), "0"))
	if err != nil {
		return nil, err
	}
	ctx, stop := context.WithTimeout(parent, 5*time.Minute)
	uploads := make(chan []byte, 1)
	done := make(chan struct{})
	host := listener.Addr().String()
	var token [16]byte
	rand.Read(token[:])
	path := "/" + hex.EncodeToString(token[:]) + "/"
	transfer := &BackupTransfer{URL: "http://" + host + path, Uploads: uploads, Done: done, stop: stop}
	var uploaded atomic.Bool
	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, err := netip.ParseAddrPort(r.RemoteAddr)
			if err != nil || !subnet.Contains(peer.Addr()) || r.Host != host {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; form-action 'self'; style-src 'unsafe-inline'; frame-ancestors 'none'")
			switch {
			case r.Method == http.MethodGet && r.URL.Path == path:
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, transferPage)
			case r.Method == http.MethodGet && r.URL.Path == path+"backup":
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
				_, _ = w.Write(data)
			case r.Method == http.MethodPost && r.URL.Path == path+"upload":
				if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
					http.Error(w, "请从互传页面上传", http.StatusForbidden)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, BackupMaxBytes+(32<<10))
				data, err := readBackupUpload(r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				payload, _, err := decodeBackup(data)
				if err != nil || len(payload.Favorites) == 0 && len(payload.Preferences) == 0 {
					http.Error(w, "文件中没有可导入的关注或偏好设置", http.StatusBadRequest)
					return
				}
				if !uploaded.CompareAndSwap(false, true) {
					http.Error(w, "本次互传已收到一份备份", http.StatusConflict)
					return
				}
				uploads <- data
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				_, _ = io.WriteString(w, "备份已发送。请回到 Live MyGo 确认导入。")
			default:
				http.NotFound(w, r)
			}
		}),
	}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	go func() {
		defer stop()
		defer close(done)
		_ = server.Serve(listener)
	}()
	return transfer, nil
}

func readBackupUpload(r *http.Request) ([]byte, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, errors.New("请选择备份文件")
	}
	part, err := reader.NextPart()
	if err != nil || part.FormName() != "backup" || part.FileName() == "" {
		return nil, errors.New("请选择备份文件")
	}
	data, err := io.ReadAll(io.LimitReader(part, BackupMaxBytes+1))
	if err != nil || len(data) > BackupMaxBytes {
		return nil, errors.New("备份文件无法读取或超过 8 MB")
	}
	_ = part.Close()
	return data, nil
}
