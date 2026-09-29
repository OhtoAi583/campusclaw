// Package kb 负责知识库的分块、向量化与索引写入。
// 本迭代的定位是"能查且可溯源"：块是检索与溯源的最小单位，块必须带字符区间。
package kb

import (
	"strings"
	"unicode"
)

// Chunk 是正文的一个片段。Start/End 是相对 knowledge_entries.content 的字符（rune）区间。
type Chunk struct {
	Index   int
	Start   int
	End     int
	Content string
}

// Split 把正文切成块。
// 策略：优先在 Markdown 标题处断开；某一段仍超过 size 时，按 size 切分并保留 overlap 个字符的重叠。
// 相邻块的重叠用于避免"答案刚好被切断"，同时块内容始终等于正文对应区间的子串。
func Split(content string, size, overlap int) []Chunk {
	if size <= 0 {
		size = 600
	}
	if overlap < 0 || overlap >= size {
		overlap = 0
	}
	runes := []rune(content)
	if len(runes) == 0 {
		return nil
	}

	// 1) 找出标题行的起点，作为优先切分点
	breaks := []int{0}
	pos := 0
	for _, line := range strings.SplitAfter(content, "\n") {
		lineRunes := len([]rune(line))
		if isHeading(line) && pos > 0 {
			breaks = append(breaks, pos)
		}
		pos += lineRunes
	}
	breaks = append(breaks, len(runes))

	// 2) 合并过短的段落，避免产生大量碎片。
	//    注意：合并必须"延长上一段的 end"，不能直接跳过——跳过会丢掉两个切分点之间的正文。
	sections := make([][2]int, 0, len(breaks))
	for i := 0; i < len(breaks)-1; i++ {
		start, end := breaks[i], breaks[i+1]
		if end <= start {
			continue
		}
		if len(sections) > 0 && start-sections[len(sections)-1][0] < size/2 {
			sections[len(sections)-1][1] = end
			continue
		}
		sections = append(sections, [2]int{start, end})
	}

	// 3) 段落仍超长时按 size 切分，保留 overlap 重叠
	var out []Chunk
	for _, sec := range sections {
		for start := sec[0]; start < sec[1]; {
			end := start + size
			if end >= sec[1] {
				end = sec[1]
			}
			out = append(out, Chunk{
				Index:   len(out),
				Start:   start,
				End:     end,
				Content: string(runes[start:end]),
			})
			if end >= sec[1] {
				break
			}
			start = end - overlap
			if start < sec[0] {
				start = sec[0]
			}
			if start >= sec[1] {
				break
			}
		}
	}
	// 4) 相邻块内容完全相同（例如短文档）时只保留一块
	deduped := out[:0]
	for i, c := range out {
		if i > 0 && c.Content == out[i-1].Content {
			continue
		}
		deduped = append(deduped, c)
	}
	for i := range deduped {
		deduped[i].Index = i
	}
	return deduped
}

func isHeading(line string) bool {
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "#") {
		return false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	return i <= 6 && i < len(trimmed) && (trimmed[i] == ' ' || trimmed[i] == '\t')
}

// Tokenize 把文本切成用于哈希嵌入的 token：
//   - 连续的 ASCII 字母数字 → 整个词
//   - 连续的 CJK 字符 → 该串的 1~3 元字组
func Tokenize(text string) []string {
	var tokens []string
	var word, cjk []rune
	flushWord := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	flushCJK := func() {
		if len(cjk) > 0 {
			for n := 1; n <= 3; n++ {
				for i := 0; i+n <= len(cjk); i++ {
					tokens = append(tokens, string(cjk[i:i+n]))
				}
			}
			cjk = cjk[:0]
		}
	}
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Han, r):
			flushWord()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return tokens
}
