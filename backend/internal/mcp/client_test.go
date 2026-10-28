package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 用仓库自带的 MCP 演示服务器跑一次真实握手：initialize → tools/list → tools/call。
func TestSessionAgainstDemoServer(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("需要 go 工具链构建 MCP 演示服务器")
	}
	bin := filepath.Join(t.TempDir(), "mcp-demo")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/mcp-demo")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建 mcp-demo 失败: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sess, err := Start(ctx, bin)
	if err != nil {
		t.Fatalf("MCP initialize 失败: %v", err)
	}
	defer sess.Close()

	tools, err := sess.ListTools(ctx)
	if err != nil {
		t.Fatalf("tools/list 失败: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("应暴露 2 个工具，实际 %d", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Name] = true
		if tl.InputSchema["type"] != "object" {
			t.Errorf("工具 %s 应带 object 类型的 inputSchema", tl.Name)
		}
		if tl.Description == "" {
			t.Errorf("工具 %s 缺少描述", tl.Name)
		}
	}
	if !names["word_count"] || !names["text_stats"] {
		t.Fatalf("工具名不符: %v", names)
	}

	out, err := sess.CallTool(ctx, "word_count", map[string]any{"text": "一二三\n\n四五"})
	if err != nil {
		t.Fatalf("tools/call 失败: %v", err)
	}
	if !strings.Contains(out, "字数：5") || !strings.Contains(out, "段落：2") {
		t.Fatalf("工具返回不符: %q", out)
	}
}

func TestCallUnknownToolReturnsError(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("需要 go 工具链")
	}
	bin := filepath.Join(t.TempDir(), "mcp-demo")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/mcp-demo").CombinedOutput(); err != nil {
		t.Fatalf("构建失败: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sess, err := Start(ctx, bin)
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	defer sess.Close()
	if _, err := sess.CallTool(ctx, "not_exists", map[string]any{}); err == nil {
		t.Fatal("未知工具应返回错误")
	}
}
