// Package answer 实现"基于知识库的问答"：先在本班检索，取得切片之后才生成回答，并标注出处。
//
// 两条硬约束（见第 4 课课件与 change 的 delta）：
//  1. 没有命中切片时**不调用生成模型**，直接返回固定文案与空 citations；
//  2. 交给模型的只有材料标题、切片序号与切片正文，绝不包含向量分量，也不含其他班级的切片。
package answer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"campusclaw/backend/internal/search"
)

// NoEvidenceText 是无命中时的固定文案。
const NoEvidenceText = "资料中未找到相关内容"

// MaxContext 是最多送进模型的切片数（课件要求取前 4 条）。
const MaxContext = 4

// Citation 是回答里的一个出处，序号与回答中的 [1]、[2] 一一对应。
type Citation struct {
	Index         int     `json:"index"`
	MaterialID    int64   `json:"material_id"`
	MaterialTitle string  `json:"material_title"`
	OriginalName  string  `json:"original_name"`
	ChunkIndex    int     `json:"chunk_index"`
	StartOffset   int     `json:"start_offset"`
	EndOffset     int     `json:"end_offset"`
	Snippet       string  `json:"snippet"`
	Score         float64 `json:"score"`
}

// Result 是一次问答的返回。
type Result struct {
	Answer      string     `json:"answer"`
	Citations   []Citation `json:"citations"`
	ModelCalled bool       `json:"model_called"`
	Engine      string     `json:"engine"`
}

// ChatClient 是生成模型的抽象。真实实现调用兼容 OpenAI 的对话网关；
// 未配置网关时使用本地摘要实现，保证离线也能演示完整链路。
type ChatClient interface {
	Name() string
	Answer(ctx context.Context, question string, chunks []search.Result) (string, error)
}

// Service 组装检索与生成两步。
type Service struct {
	Search *search.Service
	Chat   ChatClient
	Now    func() time.Time
}

// Ask 执行"先检索、再生成"。
func (s *Service) Ask(ctx context.Context, classID int64, question string, topK int) (Result, error) {
	if topK <= 0 || topK > MaxContext {
		topK = MaxContext
	}
	hits, _, err := s.Search.Search(ctx, classID, question, topK)
	if err != nil {
		return Result{}, err
	}

	// 无命中：不调用生成模型，直接用固定文案返回（课件明确要求）。
	if len(hits) == 0 {
		return Result{
			Answer:      NoEvidenceText,
			Citations:   []Citation{},
			ModelCalled: false,
			Engine:      "none",
		}, nil
	}

	citations := make([]Citation, 0, len(hits))
	for i, hit := range hits {
		citations = append(citations, Citation{
			Index:         i + 1,
			MaterialID:    hit.MaterialID,
			MaterialTitle: hit.MaterialTitle,
			OriginalName:  hit.OriginalName,
			ChunkIndex:    hit.ChunkIndex,
			StartOffset:   hit.StartOffset,
			EndOffset:     hit.EndOffset,
			Snippet:       hit.Content,
			Score:         hit.Score,
		})
	}

	answer, err := s.Chat.Answer(ctx, question, hits)
	if err != nil {
		return Result{}, fmt.Errorf("生成回答失败: %w", err)
	}
	return Result{
		Answer:      answer,
		Citations:   citations,
		ModelCalled: true,
		Engine:      s.Chat.Name(),
	}, nil
}

// BuildPrompt 组装交给生成模型的内容：材料标题 + 切片序号 + 切片正文 + 用户问题。
// 客户端自行构造的 system 消息不会进入这里——本函数只接受检索结果。
func BuildPrompt(question string, chunks []search.Result) (system string, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "问题：%s\n\n本班材料片段：\n", question)
	for i, c := range chunks {
		fmt.Fprintf(&b, "\n[%d] 《%s》（第 %d 块，字符 %d-%d）\n%s\n",
			i+1, c.MaterialTitle, c.ChunkIndex+1, c.StartOffset, c.EndOffset, strings.TrimSpace(c.Content))
	}
	b.WriteString("\n请只依据以上片段作答，不要使用片段之外的知识；每个结论后用 [编号] 标注出处；片段不足以回答时直接说明未找到。")
	return "你是 CampusClaw 的教研助手，只能依据给定的本班材料片段作答。", b.String()
}

// ExtractiveClient 是不依赖外部网关的本地实现：
// 把命中的切片按句摘出来，拼成简短回答并保留 [编号] 标注。
// 它是"取回的证据"，不是模型凭记忆编出来的内容，因此同样可溯源。
type ExtractiveClient struct{}

// Name 返回引擎名，便于前端与日志区分本次回答由谁生成。
func (ExtractiveClient) Name() string { return "local-extractive" }

// Answer 生成摘录式回答。
func (ExtractiveClient) Answer(_ context.Context, question string, chunks []search.Result) (string, error) {
	// 只用问题里的实词做一句筛选，避免把整段材料原样倒出来
	keywords := significantTokens(question)
	var b strings.Builder
	fmt.Fprintf(&b, "根据本班材料，就「%s」可以找到以下依据：\n", strings.TrimSpace(question))
	count := 0
	for i, c := range chunks {
		sentence := firstRelevantSentence(c.Content, keywords)
		if sentence == "" {
			continue
		}
		count++
		fmt.Fprintf(&b, "%d. %s [%d]\n", count, sentence, i+1)
		if count >= 3 {
			break
		}
	}
	if count == 0 {
		// 命中切片但摘不出句子时，至少给出第一条出处的开头
		fmt.Fprintf(&b, "1. %s [1]\n", firstSentence(chunks[0].Content))
	}
	b.WriteString("\n（以上内容摘自本班材料，点击出处可回到原文核对。）")
	return b.String(), nil
}

// firstRelevantSentence 返回包含任一关键词的第一句；没有关键词命中时返回首句。
func firstRelevantSentence(content string, keywords []string) string {
	sentences := splitSentences(content)
	lowerKeywords := make([]string, 0, len(keywords))
	for _, k := range keywords {
		lowerKeywords = append(lowerKeywords, strings.ToLower(k))
	}
	// 命中的句子可能不止一句：取最长的一句，避免把标题碎片当成答案。
	best := ""
	for _, s := range sentences {
		lower := strings.ToLower(s)
		for _, k := range lowerKeywords {
			if strings.Contains(lower, k) {
				if len([]rune(s)) > len([]rune(best)) {
					best = s
				}
				break
			}
		}
	}
	return best
}

func firstSentence(content string) string {
	sentences := splitSentences(content)
	if len(sentences) == 0 {
		return strings.TrimSpace(content)
	}
	return sentences[0]
}

// splitSentences 按中文与英文句末标点切句，并去掉 Markdown 标题符号。
func splitSentences(content string) []string {
	replaced := strings.NewReplacer("\n", "。", "\r", " ", "#", " ", "|", " ").Replace(content)
	fields := strings.FieldsFunc(replaced, func(r rune) bool {
		switch r {
		case '。', '！', '？', '.', '!', '?', '；', ';':
			return true
		}
		return false
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		s := strings.TrimSpace(f)
		if len([]rune(s)) < 10 { // 跳过标题与列表项这类过短的碎片
			continue
		}
		out = append(out, s)
	}
	return out
}

// significantTokens 取问题里长度 >= 2 的字词作为筛选依据。
func significantTokens(question string) []string {
	var tokens []string
	var buf []rune
	flush := func() {
		if len(buf) >= 2 {
			tokens = append(tokens, string(buf))
		}
		buf = buf[:0]
	}
	for _, r := range question {
		switch {
		case r == ' ' || r == '　' || r == '，' || r == '。' || r == '？' || r == '?' || r == '的' || r == '是':
			flush()
		default:
			buf = append(buf, r)
		}
	}
	flush()
	return tokens
}
