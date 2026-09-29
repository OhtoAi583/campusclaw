package kb

import (
	"math"
	"strings"
	"testing"
)

// 分块必须覆盖正文，且每块内容等于正文对应区间的子串（spec R4.1）。
func TestSplitCoversContent(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 标题一\n\n")
	b.WriteString(strings.Repeat("这是第一段的内容。", 60))
	b.WriteString("\n\n## 标题二\n\n")
	b.WriteString(strings.Repeat("这是第二段的内容。", 60))
	content := b.String()
	runes := []rune(content)

	chunks := Split(content, 120, 20)
	if len(chunks) < 2 {
		t.Fatalf("长正文应被切成多块，实际 %d 块", len(chunks))
	}
	covered := 0
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("块序号应从 0 连续递增，第 %d 块序号为 %d", i, c.Index)
		}
		if c.Content != string(runes[c.Start:c.End]) {
			t.Errorf("第 %d 块内容与正文区间不一致", i)
		}
		if c.Start > covered {
			t.Errorf("第 %d 块之前有未覆盖区间：%d..%d", i, covered, c.Start)
		}
		if c.End > covered {
			covered = c.End
		}
		if len([]rune(c.Content)) > 120 {
			t.Errorf("第 %d 块超过 size 上限：%d", i, len([]rune(c.Content)))
		}
	}
	if covered != len(runes) {
		t.Errorf("分块未覆盖正文全部字符：覆盖到 %d，正文长度 %d", covered, len(runes))
	}
}

// 同一正文在相同配置下重复分块必须稳定（spec R4.3）。
func TestSplitStable(t *testing.T) {
	content := "# A\n\n" + strings.Repeat("内容内容内容。", 200)
	a := Split(content, 100, 10)
	b := Split(content, 100, 10)
	if len(a) != len(b) {
		t.Fatalf("两次分块数量不同：%d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("第 %d 块不一致", i)
		}
	}
}

// 嵌入必须确定性、维度正确且 L2 归一化（spec R5.1 / R5.2）。
func TestHashingEmbedderDeterministic(t *testing.T) {
	e := NewHashingEmbedder(128)
	v1 := e.Embed("语文阅读课教学设计")
	v2 := e.Embed("语文阅读课教学设计")
	if len(v1) != 128 {
		t.Fatalf("维度应为 128，实际 %d", len(v1))
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("同一输入两次嵌入结果不同，位置 %d", i)
		}
	}
	var norm float64
	for _, x := range v1 {
		norm += float64(x) * float64(x)
	}
	if math.Abs(math.Sqrt(norm)-1) > 1e-6 {
		t.Fatalf("向量未归一化，模长 %f", math.Sqrt(norm))
	}
}

// 相关文本的相似度必须高于无关文本（spec R5.3 的基础）。
func TestEmbedderRanksRelatedHigher(t *testing.T) {
	e := NewHashingEmbedder(512)
	query := e.Embed("阅读课的导入环节怎么设计")
	related := e.Embed("语文阅读课教学设计：导入环节 5 分钟，由情境入题。")
	unrelated := e.Embed("数学课堂练习：分数四则运算与验算习惯。")
	if Dot(query, related) <= Dot(query, unrelated) {
		t.Fatalf("相关片段得分应高于无关片段：related=%f unrelated=%f",
			Dot(query, related), Dot(query, unrelated))
	}
}

// 向量编解码必须无损，且长度等于维度 × 4（spec R5.1）。
func TestVectorEncodingRoundTrip(t *testing.T) {
	e := NewHashingEmbedder(64)
	original := e.Embed("内容溯源与班级隔离")
	raw := EncodeVector(original)
	if len(raw) != 64*4 {
		t.Fatalf("编码长度应为 %d，实际 %d", 64*4, len(raw))
	}
	decoded, err := DecodeVector(raw, 64)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	for i := range original {
		if original[i] != decoded[i] {
			t.Fatalf("解码结果与原始向量不一致，位置 %d", i)
		}
	}
	if _, err := DecodeVector(raw, 32); err == nil {
		t.Fatal("维度不匹配时应返回错误")
	}
}

func TestTokenize(t *testing.T) {
	tokens := Tokenize("阅读 reading 2026")
	joined := strings.Join(tokens, "|")
	for _, want := range []string{"阅读", "reading", "2026"} {
		if !strings.Contains(joined, want) {
			t.Errorf("分词结果应包含 %q，实际 %q", want, joined)
		}
	}
}

// 回归：标题密集的短文档最容易在"合并过短段落"时丢掉正文（R4.1）。
func TestSplitKeepsContentWithDenseHeadings(t *testing.T) {
	content := "# A班 语文阅读课教学设计\n\n## 目标\n\n- 训练概括能力\n- 积累语言材料\n\n## 课堂流程\n\n| 环节 | 时长 |\n| --- | --- |\n| 导入 | 5 分钟 |\n"
	runes := []rune(content)
	chunks := Split(content, 600, 80)
	if len(chunks) == 0 {
		t.Fatal("不应产生 0 块")
	}
	covered := 0
	for _, c := range chunks {
		if c.Start > covered {
			t.Fatalf("正文在 %d..%d 之间被丢弃", covered, c.Start)
		}
		if c.End > covered {
			covered = c.End
		}
	}
	if covered != len(runes) {
		t.Fatalf("分块没有覆盖到正文末尾：覆盖 %d / 总长 %d", covered, len(runes))
	}
	// 全文必须能在块内容里找到（允许重叠，不允许缺失）
	joined := ""
	for _, c := range chunks {
		joined += c.Content
	}
	for _, must := range []string{"训练概括能力", "导入 | 5 分钟"} {
		if !strings.Contains(joined, must) {
			t.Errorf("分块结果中缺失正文片段：%q", must)
		}
	}
}
