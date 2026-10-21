#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成《CampusClaw 个人项目阶段性验收报告》PDF（reportlab）。数据来自 evidence.json。"""
import json
import os

from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (BaseDocTemplate, Frame, Image, KeepTogether, PageBreak,
                                PageTemplate, Paragraph, Preformatted, Spacer, Table, TableStyle)

HERE = os.path.dirname(os.path.abspath(__file__))
PDF = os.path.join(HERE, "个人项目阶段性验收报告.pdf")
SHOTS = os.path.join(HERE, "截图")
EV = json.load(open(os.path.join(HERE, "evidence.json"), encoding="utf-8"))

BRAND = colors.HexColor("#9c1f24")
INK = colors.HexColor("#1f1c1b")
MUTED = colors.HexColor("#6f6a67")
LINE = colors.HexColor("#e3ddd7")
CODE_BG = colors.HexColor("#f5f3f1")

pdfmetrics.registerFont(TTFont("CN", "/System/Library/Fonts/STHeiti Light.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("CNB", "/System/Library/Fonts/STHeiti Medium.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("MONO", "/System/Library/Fonts/Menlo.ttc", subfontIndex=0))

S = {
    "title": ParagraphStyle("t", fontName="CNB", fontSize=24, leading=32, textColor=BRAND, alignment=1),
    "sub": ParagraphStyle("s", fontName="CN", fontSize=12, leading=20, textColor=MUTED, alignment=1),
    "h1": ParagraphStyle("h1", fontName="CNB", fontSize=16, leading=24, textColor=BRAND, spaceBefore=8, spaceAfter=7),
    "h2": ParagraphStyle("h2", fontName="CNB", fontSize=12, leading=19, textColor=INK, spaceBefore=10, spaceAfter=4),
    "h3": ParagraphStyle("h3", fontName="CNB", fontSize=10.2, leading=16, textColor=BRAND, spaceBefore=7, spaceAfter=3),
    "body": ParagraphStyle("b", fontName="CN", fontSize=9.5, leading=15.8, textColor=INK, spaceAfter=4),
    "li": ParagraphStyle("li", fontName="CN", fontSize=9.5, leading=15.8, textColor=INK,
                         leftIndent=12, bulletIndent=3, spaceAfter=2),
    "th": ParagraphStyle("th", fontName="CNB", fontSize=8.9, leading=13, textColor=colors.white),
    "td": ParagraphStyle("td", fontName="CN", fontSize=8.9, leading=13, textColor=INK),
    "cap": ParagraphStyle("cap", fontName="CN", fontSize=8.4, leading=12.5, textColor=MUTED, spaceBefore=2, spaceAfter=7),
    "codecap": ParagraphStyle("cc", fontName="CN", fontSize=8.2, leading=12, textColor=MUTED, spaceBefore=3, spaceAfter=2),
    "concl": ParagraphStyle("concl", fontName="CN", fontSize=9.6, leading=16, textColor=INK, spaceAfter=3,
                            borderColor=BRAND, borderWidth=0, backColor=colors.HexColor("#fbf7f6"),
                            borderPadding=6, leftIndent=0),
}


def esc(t):
    return str(t).replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def inline(t):
    import re
    t = esc(t)
    t = re.sub(r"\*\*(.+?)\*\*", r"<b>\1</b>", t)
    t = re.sub(r"`(.+?)`", r"<font name='MONO' size=8.6>\1</font>", t)
    return t


def P(text, style="body"):
    return Paragraph(inline(text), S[style])


def bullets(items):
    return [Paragraph(inline(x), S["li"], bulletText="•") for x in items]


def code(text, caption=None, limit=None):
    raw_lines = str(text).replace("\t", "  ").split("\n")
    mono = "MONO" if all(ord(c) < 128 for c in "\n".join(raw_lines)) else "CN"
    font_name, font_size = (mono, 7.3)
    max_width = 156 * mm
    lines = []
    for raw in raw_lines:
        line = ""
        for ch in raw:
            if pdfmetrics.stringWidth(line + ch, font_name, font_size) > max_width and line:
                lines.append(line)
                line = "  " + ch
            else:
                line += ch
        lines.append(line)
    pre = Preformatted("\n".join(lines), ParagraphStyle("c", fontName=mono, fontSize=7.3, leading=10.2, textColor=INK))
    t = Table([[pre]], colWidths=[170 * mm])
    t.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, -1), CODE_BG), ("BOX", (0, 0), (-1, -1), 0.5, LINE),
                           ("LEFTPADDING", (0, 0), (-1, -1), 7), ("RIGHTPADDING", (0, 0), (-1, -1), 7),
                           ("TOPPADDING", (0, 0), (-1, -1), 6), ("BOTTOMPADDING", (0, 0), (-1, -1), 6)]))
    flow = [t, Spacer(1, 5)]
    return KeepTogether(([Paragraph(caption, S["codecap"])] if caption else []) + flow)


def table(header, rows, widths=None):
    data = [[Paragraph(h, S["th"]) for h in header]]
    for r in rows:
        data.append([Paragraph(inline(c), S["td"]) for c in r])
    t = Table(data, colWidths=widths or [170 * mm / len(header)] * len(header), repeatRows=1)
    t.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, 0), BRAND),
                           ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#faf8f6")]),
                           ("GRID", (0, 0), (-1, -1), 0.4, LINE), ("VALIGN", (0, 0), (-1, -1), "TOP"),
                           ("LEFTPADDING", (0, 0), (-1, -1), 5), ("RIGHTPADDING", (0, 0), (-1, -1), 5),
                           ("TOPPADDING", (0, 0), (-1, -1), 3.5), ("BOTTOMPADDING", (0, 0), (-1, -1), 3.5)]))
    return KeepTogether([t, Spacer(1, 6)])


def shot(name, caption, width=156 * mm):
    path = os.path.join(SHOTS, name)
    if not os.path.exists(path):
        return []
    img = Image(path)
    ratio = img.imageHeight / float(img.imageWidth)
    img.drawWidth = width
    img.drawHeight = width * ratio
    if img.drawHeight > 120 * mm:
        img.drawHeight = 120 * mm
        img.drawWidth = img.drawHeight / ratio
    return [KeepTogether([img, Paragraph(caption, S["cap"])])]


def footer(canvas, doc):
    canvas.saveState()
    canvas.setStrokeColor(LINE); canvas.setLineWidth(0.4)
    canvas.line(20 * mm, 14 * mm, 190 * mm, 14 * mm)
    canvas.setFont("CN", 7.5); canvas.setFillColor(MUTED)
    canvas.drawString(20 * mm, 9.5 * mm, "CampusClaw 个人项目阶段性验收报告")
    canvas.drawRightString(190 * mm, 9.5 * mm, "第 %d 页" % doc.page)
    canvas.restoreState()


def build(story):
    doc = BaseDocTemplate(PDF, pagesize=A4, leftMargin=20 * mm, rightMargin=20 * mm,
                          topMargin=16 * mm, bottomMargin=18 * mm,
                          title="CampusClaw 个人项目阶段性验收报告", author="龙浩")
    frame = Frame(doc.leftMargin, doc.bottomMargin, doc.width, doc.height, id="main")
    doc.addPageTemplates([PageTemplate(id="all", frames=[frame], onPage=footer)])
    doc.build(story)


def story():
    qa = EV["scenarios"]["qa"]
    lp = EV["scenarios"]["lesson_plan"]
    mcp_s = EV["scenarios"]["mcp"]
    qa_call = qa["calls"][0]
    lp_call = lp["calls"][0]
    mcp_calls = mcp_s["calls"]
    mcp_first, mcp_second = mcp_calls[0], mcp_calls[-1]
    s = []

    # ---------- 封面 ----------
    s += [Spacer(1, 42 * mm)]
    s += [Paragraph("CampusClaw", S["title"])]
    s += [Spacer(1, 3 * mm)]
    s += [Paragraph("个人项目阶段性验收报告", S["sub"])]
    s += [Spacer(1, 2 * mm)]
    s += [Paragraph("多 AI 应用 · 提示词模板 · 知识库检索 · Skill 技能 · MCP 工具", S["sub"])]
    s += [Spacer(1, 22 * mm)]
    s += [table(["项目", "内容"], [
        ["项目名称", "CampusClaw · 中小学教研 AI 应用平台"],
        ["仓库地址", "https://github.com/OhtoAi583/campusclaw"],
        ["项目成员", "龙浩（个人项目）"],
        ["技术栈", "Go（net/http）+ MySQL 8.0 + React 18 + Nginx + Docker Compose；LLM 网关 DeepSeek（OpenAI 兼容）"],
        ["报告日期", "2026-10-21"],
        ["验收方式", "平台对每次 LLM 调用做全量报文落库，报告中的参数与返回均取自真实运行记录"],
    ], widths=[28 * mm, 142 * mm])]
    s += [PageBreak()]

    # ---------- 1 项目简介 ----------
    s += [P("一、项目简介", "h1")]
    s += [P("CampusClaw 是一个面向中小学教研场景的 AI 应用平台。它把「班级知识库 + 提示词模板 + 技能 + 外部工具」"
            "组合成可配置的 AI 应用：教师选一个应用、输入问题，平台在**本班材料**范围内检索，"
            "按模板组装提示词、注入启用的技能、挂载 MCP 工具，再调用大模型并把出处标回原文。", "body")]
    s += [P("与验收要求逐条对照：", "h2")]
    s += [table(["验收要求", "平台实现", "验证入口"], [
        ["1. 内置多个 AI 应用", "内置 4 个应用：教研问答 / 教案生成 / 材料摘要 / 学情分析，各自绑定不同模板、技能与工具",
         "GET /api/apps；平台页应用卡片"],
        ["2. 支持提示词模板", "4 套模板（system + user + 变量），渲染后原样作为消息发出",
         "GET /api/templates；平台页「提示词模板」"],
        ["3. 支持知识库检索", "按会话班级检索切片，按相似度取 top-k；跨班内容不进入候选集",
         "POST /api/chat；meta.kb_chunks"],
        ["4. 内置 Skill + 上传技能", "3 个内置技能（引用校验 / 教研文风 / 结构化输出）；支持上传 .md 技能",
         "GET/POST /api/skills；平台页「技能」"],
        ["5. 外部工具及 MCP 接入", "MCP stdio 客户端 + 仓库自带 MCP 服务器（word_count / text_stats）",
         "GET /api/mcp/servers/{id}/tools；平台页「MCP」"],
    ], widths=[40 * mm, 86 * mm, 44 * mm])]

    s += [P("二、验收方法与取证方式", "h1")]
    s += [P("为了避免「口头说明」，平台把**每一次** LLM 调用完整落库：", "body")]
    s += bullets([
        "`llm_calls.request_json`：实际发给 `POST /chat/completions` 的完整请求体（model / messages / temperature / max_tokens / tools）；",
        "`llm_calls.response_json`：网关返回的原始响应（choices / usage / finish_reason）；",
        "`llm_calls` 同时记录 `kb_chunks`、`kb_chars`、`skill_chars`、`tool_rounds`、`round_index` 等计量字段；",
        "通过 `GET /api/llm-calls/{id}` 可随时回放某次调用的完整报文，页面「本次对话发给 LLM 的原始报文」即读取该接口。",
    ])
    s += [code('POST /api/chat\n{"app_id":"kb-qa","question":"导入环节为什么要用情境图？"}\n\n'
               'GET /api/llm-calls/{id}\n→ { request: {…完整请求体…}, response: {…网关原始返回…}, kb_chunks, kb_chars, skill_chars, tools }',
               "取证接口：对话与报文读取")]
    s += [P("下面五个问题的结论，全部引用这些真实报文；截图见第六章。", "body")]

    # ---------- 3 五个验证问题 ----------
    s += [PageBreak()]
    s += [P("三、五个验证问题", "h1")]

    # Q1
    s += [P("问题 1：每次对话传给 LLM 的具体参数是什么？返回什么？提示词模板内容是否一并传递？", "h2")]
    s += [P("**结论：模板内容会一并传递。** 平台先渲染模板，把渲染结果原样放进 `messages[0]`（system）"
            "与 `messages[1]`（user），再连同采样参数一起发给网关。", "concl")]
    s += [table(["请求字段", "本次取值", "说明"], [
        ["model", qa_call["model"], "应用配置的模型"],
        ["temperature", qa_call["temperature"], "应用配置的采样温度"],
        ["max_tokens", qa_call["max_tokens"], "应用配置的输出上限"],
        ["stream", str(qa_call["stream"]), "本阶段关闭流式"],
        ["messages", "%d 条（%s）" % (len(qa_call["messages"]), "、".join("%s %d 字" % (m["role"], m["chars"]) for m in qa_call["messages"])), "system 为渲染后的模板 + 技能；user 为模板 + 检索片段 + 问题"],
        ["tools", "无" if not qa_call["tools"] else "%d 个" % len(qa_call["tools"]), "未绑定 MCP 的应用不带此字段"],
    ], widths=[26 * mm, 64 * mm, 80 * mm])]
    s += [P("实际请求体顶层字段：`%s`" % ", ".join(qa_call["request_fields"]), "body")]
    s += [P("模板 `%s` 渲染后的 system 消息（原文，前 220 字）：" % qa["result"]["meta"]["template_id"], "body")]
    s += [code(qa_call["system"][:260], "调用 #%d · system 消息（节选）" % qa_call["id"])]
    s += [P("模板渲染后的 user 消息（节选，含检索片段与编号）：", "body")]
    s += [code(qa_call["user"][:300], "调用 #%d · user 消息（节选）" % qa_call["id"])]
    s += [P("模型返回：`finish_reason=%s`，usage=%s；回答前 160 字：" %
            (qa_call["finish_reason"], json.dumps(qa_call["usage"], ensure_ascii=False)), "body")]
    s += [code(qa_call["response_content"][:200], "调用 #%d · 模型返回内容（节选）" % qa_call["id"])]

    # Q2
    s += [P("问题 2：绑定知识库后，每次请求是否会把知识库中的全部内容传给 LLM？", "h2")]
    s += [P("**结论：不会。** 每次只传「按会话班级过滤 + 相似度排序后的前 top-k 个切片」，"
            "不是整库、也不是整篇材料。", "concl")]
    s += [table(["指标", "数值", "含义"], [
        ["班级知识库总切片", str(qa["result"]["meta"]["kb_total_chunks"]), "本班全部可检索切片数"],
        ["本次传入切片", str(qa["result"]["meta"]["kb_chunks"]), "实际写进 user 消息的切片数（应用配置 top_k=4，实际命中 3 条）"],
        ["传入片段字符数", str(qa["result"]["meta"]["kb_chars"]), "只占本班正文的一部分"],
        ["引用条数", str(len(qa["result"]["citations"])), "每条结果都能指回材料与字符区间"],
    ], widths=[46 * mm, 34 * mm, 90 * mm])]
    s += [P("机制：检索在 SQL 层就带上 `WHERE class_id = 会话班级`，再按向量相似度排序、过滤低分候选、截取 top-k。"
            "因此绑定知识库并不会把全库塞进上下文——既控制 token 成本，也避免把无关内容与跨班数据带进模型。", "body")]

    # Q3
    s += [P("问题 3：绑定技能后，请求中是否包含该技能的全部内容？", "h2")]
    s += [P("**结论：包含。** 应用启用的技能，其 SKILL.md **正文全文**会追加到 system 消息中"
            "（`## 已启用技能：<名称>` 之后），并有总长度上限 %d 字。" % 12000, "concl")]
    s += [table(["指标", "数值"], [
        ["启用技能", "、".join(qa["result"]["meta"]["skill_ids"])],
        ["注入技能字符数", str(qa["result"]["meta"]["skill_chars"])],
        ["注入位置", "system 消息末尾"],
    ], widths=[50 * mm, 120 * mm])]
    s += [P("system 消息中技能部分的原文（节选）：", "body")]
    skill_marker = qa_call["system"].find("## 已启用技能")
    s += [code(qa_call["system"][skill_marker:skill_marker + 320], "调用 #%d · system 中的技能正文（节选）" % qa_call["id"])]
    s += [P("对比：教案生成应用绑定了两个技能（引用校验 + 教研文风），system 长度从 %d 字增加到 %d 字，"
            "两个技能的全文都在请求里。" % (qa_call["messages"][0]["chars"], lp_call["messages"][0]["chars"]), "body")]

    # Q4
    s += [P("问题 4：执行具体技能时，其执行逻辑是什么？", "h2")]
    s += [P("**结论：Skill 是「提示词层面的执行约束」，不是函数调用。** 平台不执行技能里的代码，"
            "而是把技能全文注入 system，让模型按技能规定的规则生成；平台侧负责把规则落到可判定的产物上。", "concl")]
    s += [P("执行流程：", "h2")]
    s += [code("1. 技能以 SKILL.md 形式保存（内置或上传），解析出 id / 名称 / 描述 / 正文\n"
               "2. 应用通过 app_skills 绑定技能；请求时只取 enabled=1 的技能\n"
               "3. 把技能正文按 `## 已启用技能：<名称>` 拼进 system 消息（总长上限 12000 字）\n"
               "4. 模型依据技能规则生成（例如：每条结论必须带 [编号]）\n"
               "5. 平台把生成结果与检索到的 citations 一起返回，编号与出处一一对应，可回原文核对\n"
               "6. 每次调用把 system（含技能全文）、请求与返回写入 llm_calls，便于复核技能是否生效",
               "技能执行逻辑（平台侧）")]
    s += [P("以「引用校验」技能为例：它规定每条结论必须带 `[编号]`、编号必须在片段范围内、不得编造来源。"
            "本次回答实际输出为：", "body")]
    s += [code(qa["result"]["answer"][:220], "调用 #%d · 回答（带 [1][2] 编号）" % qa_call["id"])]
    s += [P("注意区分：**Skill 改变的是「模型怎么想、怎么写」；MCP 工具改变的是「模型能做什么」**（见问题 5）。", "body")]

    # Q5
    s += [P("问题 5：集成 MCP 工具后，每次请求传给 LLM 的参数是什么？", "h2")]
    s += [P("**结论：请求多了一个 `tools` 数组**（OpenAI function schema：`name` / `description` / `parameters`）。"
            "第一轮只有 system + user；模型返回 `tool_calls` 后，平台通过 MCP 执行工具，"
            "把 assistant 的 tool_calls 与 tool 结果追加进 messages，再发起第二轮请求。", "concl")]
    s += [table(["轮次", "messages 组成", "关键字段"], [
        ["第 1 轮", "%s" % "、".join("%s(%d)" % (m["role"], m["chars"]) for m in mcp_first["messages"]),
         "tools=%d 个；模型返回 %d 个 tool_calls" % (len(mcp_first["tools"]), len(mcp_first["response_tool_calls"]))],
        ["第 2 轮", "%s" % "、".join("%s(%d)" % (m["role"], m["chars"]) for m in mcp_second["messages"]),
         "追加 assistant(tool_calls) 与 tool(结果)，再取最终回答"],
    ], widths=[18 * mm, 96 * mm, 56 * mm])]
    s += [P("`tools` 数组中的工具定义（原样来自 MCP `tools/list`）：", "body")]
    tool_lines = []
    for t in mcp_first["tools"]:
        fn = t["function"]
        tool_lines.append("- %s：%s\n  parameters=%s" % (fn["name"], fn["description"],
                                                         json.dumps(fn["parameters"], ensure_ascii=False)))
    s += [code("\n".join(tool_lines), "调用 #%d · 请求中的 tools 字段" % mcp_first["id"], limit=104)]
    s += [P("第 1 轮模型返回的 tool_calls：", "body")]
    tc_lines = ["- %s(%s)" % (tc["function"]["name"], tc["function"]["arguments"][:90] + "…")
                for tc in mcp_first["response_tool_calls"]]
    s += [code("\n".join(tc_lines) or "（无）", "调用 #%d · 模型发起的工具调用" % mcp_first["id"])]
    s += [P("第二轮请求中追加的两条 tool 结果消息：", "body")]
    tool_msgs = [m for m in mcp_second["messages"] if m["role"] == "tool"]
    s += [code("\n".join("tool 结果：%s" % (m.get("content") or "")[:120] for m in tool_msgs) or "（无）",
               "调用 #%d · 工具执行结果回填" % mcp_second["id"])]
    s += [P("最终回答（节选）：", "body")]
    s += [code(mcp_s["result"]["answer"][:260], "学情分析应用 · 最终回答（节选）")]
    return s


def story_part2():
    s = []
    s += [PageBreak()]
    s += [P("四、关键设计说明", "h1")]
    s += [table(["设计点", "选择", "理由"], [
        ["模板与技能放在哪里", "渲染后作为 system / user 消息发送", "可被完整记录与复核；换成摘要式拼接会让验收无法定位"],
        ["知识库传多少", "只传 top-k 命中切片", "控制 token；避免无关内容干扰；跨班数据不进入候选集"],
        ["技能如何执行", "提示词约束 + 平台侧校验", "技能不需要可执行代码，易于上传与审阅；代价是依赖模型遵循指令"],
        ["MCP 如何接入", "stdio + JSON-RPC，工具 schema 原样转换", "不引入第三方 SDK；工具来源可审计"],
        ["报文如何留存", "每次调用写 llm_calls", "验收与排障都需要原始请求/响应，而非事后描述"],
        ["密钥管理", "只存在服务端 .env，不入库不下发", "浏览器永远看不到模型密钥"],
    ], widths=[34 * mm, 58 * mm, 78 * mm])]

    s += [P("五、截图证据", "h1")]
    s += [P("以下截图来自真实运行（Nginx → Go → MySQL / MCP / DeepSeek），完整目录见 `截图/`。", "body")]
    s += shot("01-AI应用平台-内置四个应用.png", "图 1　内置 4 个 AI 应用，各自绑定不同模板、技能与 MCP 工具")
    s += shot("03-本次对话发给LLM的完整请求与返回.png", "图 2　本次对话发给 LLM 的完整请求体与网关返回（页面直接读取 llm_calls）")
    s += shot("04-提示词模板列表与内容.png", "图 3　提示词模板：system / user 模板与变量")
    s += shot("05-技能列表-内置与上传入口.png", "图 4　技能列表：3 个内置技能 + 上传入口（.md）")
    s += shot("06-MCP服务器与工具schema.png", "图 5　MCP 服务器与 tools/list 返回的工具 schema")
    s += shot("07-MCP工具调用-两轮请求与工具结果.png", "图 6　MCP 工具调用：第一轮 tools + tool_calls，第二轮回填结果")

    s += [PageBreak()]
    s += [P("六、复现步骤", "h1")]
    s += [code("cp .env.example .env      # 填 DB / SESSION_SECRET / JWT_SECRET / CHAT_API_KEY\n"
               "docker compose up --build -d\n"
               "# 打开 http://localhost:8080 → 登录 teacher_a → 顶部「AI 应用」\n"
               "\n"
               "# 命令行复现（本报告数据即由此产生）\n"
               "python3 homework/1028-个人项目阶段性验收/collect_evidence.py\n"
               "python3 homework/1028-个人项目阶段性验收/build_report.py",
               "从零复现")]
    s += [P("七、小结", "h1")]
    s += [P("平台已具备验收要求的五项能力，并且把「到底给模型发了什么」变成了可回放的事实："
            "模板会随请求一并发送、知识库只传 top-k 命中切片而不是全库、技能正文会完整注入 system 消息、"
            "技能以提示词约束的方式执行、MCP 工具以 function schema 出现在请求的 tools 字段并通过两轮消息完成调用。", "body")]
    return s


def main():
    build(story() + story_part2())
    print("已生成:", PDF)


if __name__ == "__main__":
    main()
