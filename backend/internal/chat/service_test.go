package chat

import (
	"strings"
	"testing"

	"campusclaw/backend/internal/search"
)

func TestRender(t *testing.T) {
	tpl := "你是 {{app_name}}，服务于 {{class_name}}。问题：{{question}}"
	got := render(tpl, map[string]string{"app_name": "教研问答", "class_name": "A", "question": "导入怎么做"})
	want := "你是 教研问答，服务于 A。问题：导入怎么做"
	if got != want {
		t.Fatalf("render 结果不符\n got: %s\nwant: %s", got, want)
	}
	// 未提供的变量原样保留，便于发现模板与变量不一致
	if out := render("x{{missing}}y", map[string]string{}); out != "x{{missing}}y" {
		t.Fatalf("未提供的变量应原样保留，实际 %q", out)
	}
}

func TestBuildContextFormatsCitations(t *testing.T) {
	hits := []search.Result{
		{MaterialID: 7, MaterialTitle: "A班语文", OriginalName: "a.md", ChunkIndex: 1,
			StartOffset: 320, EndOffset: 588, Content: "导入环节用情境图引发预测。", Score: 0.31},
		{MaterialID: 9, MaterialTitle: "A班数学", OriginalName: "b.md", ChunkIndex: 0,
			StartOffset: 0, EndOffset: 120, Content: "分数四则运算练习。", Score: 0.22},
	}
	ctx, citations := buildContext(hits)

	if !strings.Contains(ctx, "[1] 《A班语文》") || !strings.Contains(ctx, "[2] 《A班数学》") {
		t.Fatalf("上下文中应带编号与材料标题：\n%s", ctx)
	}
	if !strings.Contains(ctx, "第 2 块") || !strings.Contains(ctx, "字符 320-588") {
		t.Fatalf("上下文中应带块序号与字符区间：\n%s", ctx)
	}
	if len(citations) != 2 {
		t.Fatalf("应返回 2 条出处，实际 %d", len(citations))
	}
	if citations[0].Index != 1 || citations[1].Index != 2 {
		t.Fatalf("出处编号应与 [n] 一致，实际 %+v", citations)
	}
	if citations[0].StartOffset != 320 || citations[0].EndOffset != 588 {
		t.Fatalf("出处应保留字符区间，实际 %+v", citations[0])
	}
}

func TestBuildContextEmpty(t *testing.T) {
	ctx, citations := buildContext(nil)
	if !strings.Contains(ctx, "没有检索到相关片段") {
		t.Fatalf("无命中时应给出明确占位文本，实际 %q", ctx)
	}
	if len(citations) != 0 {
		t.Fatalf("无命中时出处应为空，实际 %d 条", len(citations))
	}
}

func TestSplitArgs(t *testing.T) {
	if got := splitArgs(""); len(got) != 0 {
		t.Fatalf("空参数应返回空切片，实际 %v", got)
	}
	if got := splitArgs("--port 8080  --verbose"); len(got) != 3 || got[0] != "--port" || got[1] != "8080" || got[2] != "--verbose" {
		t.Fatalf("参数切分不符，实际 %v", got)
	}
}

func TestMaxInt(t *testing.T) {
	if maxInt(3, 5) != 5 || maxInt(9, 2) != 9 {
		t.Fatal("maxInt 结果不符")
	}
}
