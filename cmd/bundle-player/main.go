// 将播放器及其动态库复制进 .app，改写加载路径，供独立运行。
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type library struct {
	source, name string
	refs         map[string]string
}

func run(command string, args ...string) string {
	b, err := exec.Command(command, args...).CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("%s: %v\n%s", command, err, b))
	}
	return string(b)
}

func copyFile(source, destination string) {
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		panic(err)
	}
	in, err := os.Open(source)
	if err != nil {
		panic(err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(err)
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		panic(err)
	}
	if err = out.Close(); err != nil {
		panic(err)
	}
}

func system(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/") || strings.HasPrefix(path, "/System/")
}

func resolve(ref, parent string) string {
	if system(ref) {
		return ""
	}
	paths := []string{ref}
	if strings.HasPrefix(ref, "@loader_path/") {
		paths = []string{filepath.Join(filepath.Dir(parent), strings.TrimPrefix(ref, "@loader_path/"))}
	}
	if strings.HasPrefix(ref, "@rpath/") {
		name := strings.TrimPrefix(ref, "@rpath/")
		paths = []string{filepath.Join(filepath.Dir(parent), name)}
		lines := strings.Split(run("otool", "-l", parent), "\n")
		for i, line := range lines {
			if strings.TrimSpace(line) == "cmd LC_RPATH" {
				for j := i + 1; j < min(i+5, len(lines)); j++ {
					v := strings.TrimSpace(lines[j])
					if strings.HasPrefix(v, "path ") {
						base := strings.Split(strings.TrimPrefix(v, "path "), " (offset")[0]
						base = strings.ReplaceAll(base, "@loader_path", filepath.Dir(parent))
						paths = append(paths, filepath.Join(base, name))
					}
				}
			}
		}
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			real, err := filepath.EvalSymlinks(path)
			if err == nil {
				return real
			}
		}
	}
	panic("无法定位播放器依赖：" + ref)
}

func minimumOS(path string) string {
	version := "0.0"
	active := false
	for _, line := range strings.Split(run("otool", "-l", path), "\n") {
		v := strings.TrimSpace(line)
		if strings.HasPrefix(v, "Load command ") {
			active = false
		}
		if v == "cmd LC_BUILD_VERSION" || v == "cmd LC_VERSION_MIN_MACOSX" {
			active = true
		}
		if active && (strings.HasPrefix(v, "minos ") || strings.HasPrefix(v, "version ")) {
			parts := strings.Fields(v)
			if len(parts) > 1 {
				version = parts[1]
				break
			}
		}
	}
	return version
}

func newer(a, b string) bool {
	var x, y, z, p, q, r int
	fmt.Sscanf(a, "%d.%d.%d", &x, &y, &z)
	fmt.Sscanf(b, "%d.%d.%d", &p, &q, &r)
	if x != p {
		return x > p
	}
	if y != q {
		return y > q
	}
	return z > r
}

func main() {
	bundle := "build/LiveMyGo.app"
	if len(os.Args) > 1 {
		bundle = os.Args[1]
	}
	source := os.Getenv("LIVE_MYGO_LIBMPV")
	if source == "" {
		for _, path := range []string{"/opt/homebrew/opt/mpv/lib/libmpv.2.dylib", "/usr/local/opt/mpv/lib/libmpv.2.dylib"} {
			if _, err := os.Stat(path); err == nil {
				source = path
				break
			}
		}
	}
	if source == "" {
		panic("未找到 libmpv；请安装 mpv，或设置 LIVE_MYGO_LIBMPV")
	}
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		panic(err)
	}
	libraries := map[string]*library{}
	names := map[string]string{}
	queue := []string{source}
	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		if libraries[path] != nil {
			continue
		}
		name := filepath.Base(path)
		if old := names[name]; old != "" && old != path {
			panic("动态库名称冲突：" + name)
		}
		names[name] = path
		entry := &library{source: path, name: name, refs: map[string]string{}}
		libraries[path] = entry
		lines := strings.Split(run("otool", "-L", path), "\n")
		for _, line := range lines[1:] {
			ref := strings.TrimSpace(strings.Split(line, " (compatibility")[0])
			if ref == "" || system(ref) {
				continue
			}
			child := resolve(ref, path)
			if child == "" || child == path {
				continue
			}
			entry.refs[ref] = child
			queue = append(queue, child)
		}
	}
	frameworks := filepath.Join(bundle, "Contents/Frameworks")
	resources := filepath.Join(bundle, "Contents/Resources")
	minimum := minimumOS(filepath.Join(bundle, "Contents/MacOS/live-mygo"))
	var ordered []string
	for path := range libraries {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	var notices strings.Builder
	for _, path := range ordered {
		entry := libraries[path]
		target := filepath.Join(frameworks, entry.name)
		copyFile(entry.source, target)
		run("install_name_tool", "-id", "@rpath/"+entry.name, target)
		var refs []string
		for ref := range entry.refs {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		for _, ref := range refs {
			run("install_name_tool", "-change", ref, "@loader_path/"+libraries[entry.refs[ref]].name, target)
		}
		run("codesign", "--force", "--sign", "-", target)
		if version := minimumOS(path); newer(version, minimum) {
			minimum = version
		}
		fmt.Fprintf(&notices, "%s\n  %s\n", entry.name, entry.source)
		// Homebrew 将源许可证保留在 keg 中；保留可找到的原始许可证。
		for parent := filepath.Dir(entry.source); parent != "/"; parent = filepath.Dir(parent) {
			if _, err := os.Stat(filepath.Join(parent, "INSTALL_RECEIPT.json")); err != nil {
				continue
			}
			formula := filepath.Base(filepath.Dir(parent))
			filepath.WalkDir(parent, func(file string, d os.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if d.IsDir() {
					rel, _ := filepath.Rel(parent, file)
					if rel != "." && !strings.HasPrefix(rel, "share/doc") && rel != "share" {
						return filepath.SkipDir
					}
					return nil
				}
				name := strings.ToUpper(d.Name())
				if strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "COPYING") || strings.HasPrefix(name, "COPYRIGHT") || name == "NOTICE" {
					rel, _ := filepath.Rel(parent, file)
					copyFile(file, filepath.Join(resources, "licenses", formula, rel))
				}
				return nil
			})
			break
		}
	}
	if err = os.WriteFile(filepath.Join(resources, "linked-libraries.txt"), []byte(notices.String()), 0644); err != nil {
		panic(err)
	}
	for _, path := range []string{"/opt/homebrew/etc/ca-certificates/cert.pem", "/usr/local/etc/ca-certificates/cert.pem"} {
		if _, err = os.Stat(path); err == nil {
			copyFile(path, filepath.Join(resources, "cacert.pem"))
			break
		}
	}
	run("plutil", "-replace", "LSMinimumSystemVersion", "-string", minimum, filepath.Join(bundle, "Contents/Info.plist"))
	fmt.Printf("已内置 %d 个播放器动态库；最低系统版本 %s。\n", len(libraries), minimum)
}
