package server

import "testing"

func TestParseSkillMetaFrontmatter(t *testing.T) {
	body := `---
name: 引用校验（增强）
description: 强制每条结论带编号
---

# 引用校验

正文...
`
	name, desc := parseSkillMeta("cite.md", body)
	if name != "引用校验（增强）" {
		t.Fatalf("应优先读取 frontmatter 的 name，实际 %q", name)
	}
	if desc != "强制每条结论带编号" {
		t.Fatalf("应优先读取 frontmatter 的 description，实际 %q", desc)
	}
}

func TestParseSkillMetaHeadingFallback(t *testing.T) {
	body := "# 教案审阅\n\n检查教学目标是否可测量。\n\n## 规则\n- ...\n"
	name, desc := parseSkillMeta("lesson-review.md", body)
	if name != "教案审阅" {
		t.Fatalf("无 frontmatter 时应取一级标题，实际 %q", name)
	}
	if desc != "检查教学目标是否可测量。" {
		t.Fatalf("描述应取首个正文段落，实际 %q", desc)
	}
}

func TestParseSkillMetaFallbackToFilename(t *testing.T) {
	name, desc := parseSkillMeta("my-skill.md", "只有正文，没有标题也没有 frontmatter。")
	if name != "my-skill" {
		t.Fatalf("无标题时应退回文件名，实际 %q", name)
	}
	if desc == "" {
		t.Fatal("描述不应为空")
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"My Skill.md":     "my-skill-md",
		"引用 校验":           "",
		"a__b-c":          "a__b-c",
		"---weird---name": "weird-name",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q，期望 %q", in, got, want)
		}
	}
}
