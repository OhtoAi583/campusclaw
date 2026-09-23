package materials

import (
	"path/filepath"
	"strings"
	"testing"
)

// 扩展名白名单：黑名单无法穷举，双扩展名必须被挡住（R4.1）。
func TestAllowedExtensions(t *testing.T) {
	cases := map[string]bool{
		"lesson.md":      true,
		"notes.txt":      true,
		"NOTES.TXT":      true,
		"payload.exe":    false,
		"payload.md.exe": false,
		"archive.tar.gz": false,
		"noextension":    false,
	}
	for name, want := range cases {
		ext := strings.ToLower(filepath.Ext(name))
		_, got := allowedExtensions[ext]
		if got != want {
			t.Errorf("%s 的判定应为 %v，实际 %v", name, want, got)
		}
	}
}

// 存储名由服务端生成，客户端文件名里的路径分隔符不能影响落盘位置（R4.3）。
func TestNewStoredNameIsGenerated(t *testing.T) {
	name, err := newStoredName(".md")
	if err != nil {
		t.Fatalf("生成存储名失败: %v", err)
	}
	if strings.ContainsAny(name, `/\`) {
		t.Fatalf("存储名不应包含路径分隔符: %q", name)
	}
	if !strings.HasSuffix(name, ".md") {
		t.Fatalf("存储名应保留白名单扩展名: %q", name)
	}
	other, err := newStoredName(".md")
	if err != nil {
		t.Fatalf("生成存储名失败: %v", err)
	}
	if name == other {
		t.Fatal("两次生成的存储名不应相同")
	}
}

func TestTitleFromFilenameUsesBaseName(t *testing.T) {
	cases := map[string]string{
		"语文阅读.md":              "语文阅读",
		"../../etc/passwd.txt": "passwd",
		"lesson.markdown":      "lesson",
		".txt":                 ".txt",
	}
	for in, want := range cases {
		if got := titleFromFilename(in); got != want {
			t.Errorf("titleFromFilename(%q) = %q，期望 %q", in, got, want)
		}
	}
}
