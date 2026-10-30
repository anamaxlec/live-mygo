package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const runtimeURL = "https://github.com/shinchiro/mpv-winbuild-cmake/releases/download/20261021/mpv-dev-x86_64-20261021-git-e471496c56.7z"
const runtimeDigest = "efc719eff3dc5954b4a5efc800632808bcec01cc9034940dd14e5eb7fe9ea633"

func main() {
	archive := flag.String("runtime", "", "已下载的固定版本 libmpv 7z 文件")
	sample := flag.String("sample", "", "随测试包附带的本地 MP4 视频")
	test := flag.Bool("test", false, "打包前运行本机测试")
	flag.Parse()
	if *test {
		run(nil, "go", "test", "./...")
	}
	version, err := os.ReadFile("VERSION")
	must(err)
	name := "LiveMyGo-v" + strings.TrimSpace(string(version)) + "-windows-amd64"
	out := "build/windows-amd64"
	must(os.MkdirAll(out, 0755))
	must(os.MkdirAll("build/releases", 0755))
	fmt.Println("生成 Windows 图标与 DPI 资源")
	host := []string{"GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH, "CGO_ENABLED=0"}
	run(host, "go", "run", "./cmd/winicon", "-out", filepath.Join(out, "AppIcon.ico"))
	run(host, "go", "run", "github.com/akavel/rsrc@v0.10.2", "-arch", "amd64", "-ico", filepath.Join(out, "AppIcon.ico"), "-manifest", "scripts/windows.manifest", "-o", "rsrc_windows_amd64.syso")
	fmt.Println("编译 Windows x64 测试版")
	run([]string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}, "go", "build", "-trimpath", "-ldflags=-H=windowsgui", "-o", filepath.Join(out, "LiveMyGo.exe"), ".")
	if *archive == "" {
		*archive = "build/cache/mpv-dev-x86_64-20261021.7z"
		if _, err := os.Stat(*archive); err != nil {
			fmt.Println("下载 Windows 播放运行库")
			download(runtimeURL, *archive)
		}
	}
	actual := digest(*archive)
	if actual != runtimeDigest {
		panic("libmpv 下载文件与上游 SHA-256 不匹配")
	}
	extract, err := os.MkdirTemp("build", ".windows-runtime-")
	must(err)
	defer os.RemoveAll(extract)
	run(nil, "tar", "-xf", *archive, "-C", extract, "libmpv-2.dll")
	copyFile(filepath.Join(extract, "libmpv-2.dll"), filepath.Join(out, "libmpv-2.dll"))
	fmt.Println("准备证书、文档与许可证")
	download("https://curl.se/ca/cacert.pem", filepath.Join(out, "cacert.pem"))
	copyFile("LICENSE", filepath.Join(out, "LICENSE"))
	copyFile("THIRD_PARTY.md", filepath.Join(out, "THIRD_PARTY.md"))
	copyFile("assets/icons/LICENSE", filepath.Join(out, "licenses/Lucide-LICENSE"))
	copyGoLicenses(out)
	download("https://raw.githubusercontent.com/mpv-player/mpv/e471496c56/LICENSE.GPL", filepath.Join(out, "licenses/libmpv/LICENSE.GPL"))
	download("https://raw.githubusercontent.com/mpv-player/mpv/e471496c56/LICENSE.LGPL", filepath.Join(out, "licenses/libmpv/LICENSE.LGPL"))
	copyTree("docs/windows", filepath.Join(out, "docs/windows"))
	readme := "# Live MyGo " + strings.TrimSpace(string(version)) + " 测试版\n\n解压后运行 LiveMyGo.exe，保留同目录的 libmpv-2.dll 和 cacert.pem。\n\n"
	if *sample != "" {
		if filepath.Clean(*sample) != filepath.Join(out, "test-pattern.mp4") {
			copyFile(*sample, filepath.Join(out, "test-pattern.mp4"))
		}
		readme += "可以先用附带的本地测试视频检查画面和控件：\n\n```powershell\n$env:LIVE_MYGO_DATA_DIR = Join-Path $env:TEMP 'live-mygo-windows-qa'\n.\\LiveMyGo.exe -file .\\test-pattern.mp4 -mute -report .\\reports\\windows-local.json\n```\n\n"
	}
	readme += "[实机测试交接](docs/windows/README.md) · [设计准则](docs/DESIGN.md) · [MyGo 文档](docs/MYGO.md)\n\n源码位于同版本的 source.zip。当前已交叉编译，Windows 实机运行待验证。\n"
	must(os.WriteFile(filepath.Join(out, "README-Windows.md"), []byte(readme), 0644))
	for _, doc := range []string{"DESIGN.md", "MYGO.md"} {
		copyFile(filepath.Join("docs", doc), filepath.Join(out, "docs", doc))
	}
	copyTree("docs/mygo", filepath.Join(out, "docs/mygo"))
	must(os.WriteFile(filepath.Join(out, "PLAYER-SOURCE.txt"), []byte("Windows libmpv\n"+runtimeURL+"\nSHA-256: "+runtimeDigest+"\nBuild scripts: https://github.com/shinchiro/mpv-winbuild-cmake\nmpv source: https://github.com/mpv-player/mpv/tree/e471496c56\nFFmpeg source: https://git.ffmpeg.org/ffmpeg.git\n"), 0644))
	fmt.Println("打包应用与源码交接包")
	appZip := filepath.Join("build/releases", name+".zip")
	stage, err := os.MkdirTemp("build", ".windows-package-")
	must(err)
	defer os.RemoveAll(stage)
	for _, file := range []string{"LiveMyGo.exe", "libmpv-2.dll", "cacert.pem", "AppIcon.ico", "LICENSE", "THIRD_PARTY.md", "README-Windows.md", "PLAYER-SOURCE.txt"} {
		copyFile(filepath.Join(out, file), filepath.Join(stage, file))
	}
	copyTree(filepath.Join(out, "licenses"), filepath.Join(stage, "licenses"))
	for _, doc := range []string{"DESIGN.md", "MYGO.md"} {
		copyFile(filepath.Join("docs", doc), filepath.Join(stage, "docs", doc))
	}
	for _, dir := range []string{"windows", "mygo"} {
		copyTree(filepath.Join("docs", dir), filepath.Join(stage, "docs", dir))
	}
	if *sample != "" {
		copyFile(filepath.Join(out, "test-pattern.mp4"), filepath.Join(stage, "test-pattern.mp4"))
	}
	zipTree(stage, appZip, name)
	sourceZip := filepath.Join("build/releases", "LiveMyGo-v"+strings.TrimSpace(string(version))+"-source.zip")
	zipSource(sourceZip, "live-mygo")
	checks := fmt.Sprintf("%s  %s\n%s  %s\n", digest(appZip), filepath.Base(appZip), digest(sourceZip), filepath.Base(sourceZip))
	must(os.WriteFile(filepath.Join("build/releases", name+".sha256"), []byte(checks), 0644))
	fmt.Println(appZip)
	fmt.Println(sourceZip)
}

func run(env []string, program string, args ...string) {
	c := exec.Command(program, args...)
	c.Env = append(os.Environ(), env...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	must(c.Run())
}

func copyGoLicenses(out string) {
	data, err := exec.Command("go", "list", "-m", "-json", "all").Output()
	must(err)
	decode := json.NewDecoder(bytes.NewReader(data))
	for {
		var module struct {
			Path, Version, Dir string
			Main               bool
		}
		err := decode.Decode(&module)
		if err == io.EOF {
			break
		}
		must(err)
		if module.Main || module.Dir == "" {
			continue
		}
		for _, pattern := range []string{"LICENSE*", "COPYING*", "NOTICE*"} {
			files, err := filepath.Glob(filepath.Join(module.Dir, pattern))
			must(err)
			for _, file := range files {
				info, err := os.Stat(file)
				must(err)
				if info.Mode().IsRegular() {
					copyFile(file, filepath.Join(out, "licenses/go", filepath.FromSlash(module.Path), filepath.Base(file)))
				}
			}
		}
	}
	root, err := exec.Command("go", "env", "GOROOT").Output()
	must(err)
	copyFile(filepath.Join(strings.TrimSpace(string(root)), "LICENSE"), filepath.Join(out, "licenses/Go-LICENSE"))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func digest(path string) string {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	h := sha256.New()
	_, err = io.Copy(h, f)
	must(err)
	return hex.EncodeToString(h.Sum(nil))
}
func download(url, path string) {
	must(os.MkdirAll(filepath.Dir(path), 0755))
	client := http.Client{Timeout: 2 * time.Minute}
	response, err := client.Get(url)
	must(err)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("下载失败：HTTP %d", response.StatusCode))
	}
	f, err := os.Create(path)
	must(err)
	_, err = io.Copy(f, response.Body)
	closeErr := f.Close()
	must(err)
	must(closeErr)
}
func copyFile(source, target string) {
	f, err := os.Open(source)
	must(err)
	defer f.Close()
	must(os.MkdirAll(filepath.Dir(target), 0755))
	out, err := os.Create(target)
	must(err)
	_, err = io.Copy(out, f)
	closeErr := out.Close()
	must(err)
	must(closeErr)
}
func copyTree(source, target string) {
	must(filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			copyFile(path, filepath.Join(target, rel))
		}
		return nil
	}))
}
func addZip(w *zip.Writer, path, name string) {
	f, err := os.Open(path)
	must(err)
	defer f.Close()
	info, err := f.Stat()
	must(err)
	h, err := zip.FileInfoHeader(info)
	must(err)
	h.Name = filepath.ToSlash(name)
	h.Method = zip.Deflate
	out, err := w.CreateHeader(h)
	must(err)
	_, err = io.Copy(out, f)
	must(err)
}
func zipTree(root, target, prefix string) {
	f, err := os.Create(target)
	must(err)
	w := zip.NewWriter(f)
	must(filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			addZip(w, path, filepath.Join(prefix, rel))
		}
		return nil
	}))
	must(w.Close())
	must(f.Close())
}
func zipSource(target, prefix string) {
	f, err := os.Create(target)
	must(err)
	w := zip.NewWriter(f)
	for _, name := range []string{"main.go", "version.go", "go.mod", "go.sum", "VERSION", "README.md", "LICENSE", "THIRD_PARTY.md", ".gitignore"} {
		addZip(w, name, filepath.Join(prefix, name))
	}
	for _, root := range []string{"assets", "cmd", "docs", "internal", "scripts"} {
		must(filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode().IsRegular() && filepath.ToSlash(path) != "docs/DOWNBIT-STUDY.md" && filepath.Base(path) != ".DS_Store" {
				addZip(w, path, filepath.Join(prefix, path))
			}
			return nil
		}))
	}
	must(w.Close())
	must(f.Close())
}
