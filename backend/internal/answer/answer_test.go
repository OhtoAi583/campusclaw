package answer

import (
	"context"
	"strings"
	"testing"

	"campusclaw/backend/internal/search"
)

func chunks() []search.Result {
	return []search.Result{
		{MaterialID: 1, MaterialTitle: "A班语文", OriginalName: "a.md", ChunkIndex: 1,
			StartOffset: 320, EndOffset: 588,
			Content: "## 导入环节的设计意图\n导入环节用一张生活情境图引发预测，目的是把学生的注意力拉到内容走向上。\n教师只提问不评价。"},
		{MaterialID: 2, MaterialTitle: "A班数学", OriginalName: "b.md", ChunkIndex: 0,
			StartOffset: 0, EndOffset: 100, Content: "分数四则运算练习，重点训练验算习惯。"},
	}
}

// 交给模型的内容只能是标题、切片序号与正文，不能夹带向量或其他内部字段。
func TestBuildPromptOnlyCarriesEvidence(t *testing.T) {
	system, user := BuildPrompt("导入环节怎么设计", chunks())
	if !strings.Contains(system, "只能依据") {
		t.Fatalf("system 应约束只依据材料作答，实际 %q", system)
	}
	for _, want := range []string{"问题：导入环节怎么设计", "[1] 《A班语文》", "第 2 块", "字符 320-588", "情境图"} {
		if !strings.Contains(user, want) {
			t.Errorf("user 提示词缺少 %q：\n%s", want, user)
		}
	}
	for _, forbidden := range []string{"embedding", "vector", "stored_name", "/Users/", "score"} {
		if strings.Contains(strings.ToLower(user), forbidden) {
			t.Errorf("user 提示词不应包含内部字段 %q", forbidden)
		}
	}
}

func TestExtractiveAnswerCitesEveryPoint(t *testing.T) {
	client := ExtractiveClient{}
	out, err := client.Answer(context.Background(), "导入环节为什么用情境图", chunks())
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if !strings.Contains(out, "[1]") {
		t.Fatalf("回答应带 [1] 标注：\n%s", out)
	}
	if !strings.Contains(out, "情境图") {
		t.Fatalf("回答应摘出命中句子：\n%s", out)
	}
	if strings.Contains(out, "分数四则运算") {
		t.Fatalf("回答不应混入不相关切片的内容：\n%s", out)
	}
}

func TestExtractiveClientName(t *testing.T) {
	if got := (ExtractiveClient{}).Name(); got != "local-extractive" {
		t.Fatalf("引擎名不符: %q", got)
	}
}

func TestNoEvidenceText(t *testing.T) {
	if NoEvidenceText != "资料中未找到相关内容" {
		t.Fatalf("无依据文案被改动：%q", NoEvidenceText)
	}
	if MaxContext != 4 {
		t.Fatalf("课件要求最多取前 4 条切片，实际 %d", MaxContext)
	}
}

func TestSignificantTokens(t *testing.T) {
	tokens := significantTokens("导入环节为什么用情境图")
	for _, tok := range tokens {
		if len([]rune(tok)) < 2 {
			t.Fatalf("不应保留单字 token: %q", tok)
		}
	}
	joined := strings.Join(tokens, "|")
	if !strings.Contains(joined, "导入环节") {
		t.Fatalf("应保留多字词，实际 %q", joined)
	}
}
