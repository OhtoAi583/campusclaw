#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""生成《CampusClaw 身份认证与知识库问答实现方案》PDF。

代码片段直接从仓库源码抽取（按函数名定位），保证文档与代码一致。
"""
import os
import re
import sys

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle, getSampleStyleSheet
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (BaseDocTemplate, Frame, Image, KeepTogether, PageBreak,
                                PageTemplate, Paragraph, Preformatted, Spacer, Table, TableStyle)

REPO = "/Users/ohtoai583/project/campusclaw"
OUT = os.path.join(REPO, "homework/0930-课后任务-实现方案/作业-0930-身份认证与知识库问答实现方案.pdf")
SHOT_TOKEN = os.path.join(REPO, "homework/0930-课堂任务-token方案实现登录认证/截图")
SHOT_ASK = os.path.join(REPO, "homework/0930-课堂任务-基于知识库的AI问答/截图")

BRAND = colors.HexColor("#9c1f24")
INK = colors.HexColor("#1f1c1b")
MUTED = colors.HexColor("#6f6a67")
LINE = colors.HexColor("#e3ddd7")
CODE_BG = colors.HexColor("#f5f3f1")

pdfmetrics.registerFont(TTFont("CN", "/System/Library/Fonts/STHeiti Light.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("CNB", "/System/Library/Fonts/STHeiti Medium.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("MONO", "/System/Library/Fonts/Menlo.ttc", subfontIndex=0))

ss = getSampleStyleSheet()
def style(name, **kw):
    base = kw.pop("parent", ss["BodyText"])
    return ParagraphStyle(name, parent=base, **kw)

S = {
    "title": style("title", fontName="CNB", fontSize=26, leading=34, textColor=BRAND, alignment=TA_CENTER),
    "subtitle": style("subtitle", fontName="CN", fontSize=13, leading=22, textColor=MUTED, alignment=TA_CENTER),
    "h1": style("h1", fontName="CNB", fontSize=16, leading=24, textColor=BRAND, spaceBefore=10, spaceAfter=6),
    "h2": style("h2", fontName="CNB", fontSize=12.5, leading=20, textColor=INK, spaceBefore=9, spaceAfter=4),
    "body": style("body", fontName="CN", fontSize=9.6, leading=16.5, textColor=INK, spaceAfter=5),
    "bullet": style("bullet", fontName="CN", fontSize=9.6, leading=16.5, textColor=INK,
                    leftIndent=10, bulletIndent=2, spaceAfter=2),
    "caption": style("caption", fontName="CN", fontSize=8.6, leading=13, textColor=MUTED, spaceBefore=2, spaceAfter=8),
    "codecap": style("codecap", fontName="CN", fontSize=8.4, leading=12, textColor=MUTED, spaceBefore=4, spaceAfter=2),
    "cell": style("cell", fontName="CN", fontSize=9, leading=13.5, textColor=INK),
    "cellhead": style("cellhead", fontName="CNB", fontSize=9, leading=13.5, textColor=colors.white),
}


def esc(t):
    return (t.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;"))


def code_lines(rel, start_pat, end_pat=r"^\}", max_lines=70):
    """从仓库文件里按起始正则抽取代码（默认抽到列 0 的右花括号）。"""
    path = os.path.join(REPO, rel)
    lines = open(path, encoding="utf-8").read().split("\n")
    start = next((i for i, l in enumerate(lines) if re.search(start_pat, l)), None)
    if start is None:
        raise SystemExit("找不到代码片段：%s / %s" % (rel, start_pat))
    end = start
    for j in range(start + 1, len(lines)):
        if re.match(end_pat, lines[j]):
            end = j
            break
    body = lines[start:end + 1]
    if len(body) > max_lines:
        body = body[:max_lines] + ["    // …（此处省略，完整实现见仓库源码）"]
    return "\n".join(body)


def code_block(text, caption):
    mono = "MONO" if all(ord(c) < 128 for c in text) else "CN"
    wrapped = []
    limit = 118 if mono == "MONO" else 108
    for raw in text.split("\n"):
        line = raw.replace("\t", "    ")
        if len(line) <= limit:
            wrapped.append(line)
            continue
        while len(line) > limit:
            wrapped.append(line[:limit])
            line = "        " + line[limit:]
        wrapped.append(line)
    body = Preformatted("\n".join(wrapped), ParagraphStyle(
        "code", fontName=mono, fontSize=7.4, leading=10.4, textColor=INK))
    t = Table([[body]], colWidths=[170 * mm])
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, -1), CODE_BG),
        ("BOX", (0, 0), (-1, -1), 0.5, LINE),
        ("LEFTPADDING", (0, 0), (-1, -1), 7),
        ("RIGHTPADDING", (0, 0), (-1, -1), 7),
        ("TOPPADDING", (0, 0), (-1, -1), 6),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 6),
    ]))
    return KeepTogether([Paragraph(caption, S["codecap"]), t, Spacer(1, 5)])


def table(header, rows, widths=None):
    data = [[Paragraph(h, S["cellhead"]) for h in header]]
    for r in rows:
        data.append([Paragraph(str(c), S["cell"]) for c in r])
    t = Table(data, colWidths=widths or [170 * mm / len(header)] * len(header), repeatRows=1)
    t.setStyle(TableStyle([
        ("BACKGROUND", (0, 0), (-1, 0), BRAND),
        ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#faf8f6")]),
        ("GRID", (0, 0), (-1, -1), 0.4, LINE),
        ("VALIGN", (0, 0), (-1, -1), "TOP"),
        ("LEFTPADDING", (0, 0), (-1, -1), 6),
        ("RIGHTPADDING", (0, 0), (-1, -1), 6),
        ("TOPPADDING", (0, 0), (-1, -1), 4),
        ("BOTTOMPADDING", (0, 0), (-1, -1), 4),
    ]))
    return KeepTogether([t, Spacer(1, 7)])


def shot(path, caption, width=150 * mm):
    img = Image(path)
    ratio = img.imageHeight / float(img.imageWidth)
    img.drawWidth = width
    img.drawHeight = width * ratio
    if img.drawHeight > 110 * mm:
        img.drawHeight = 110 * mm
        img.drawWidth = img.drawHeight / ratio
    return KeepTogether([img, Paragraph(caption, S["caption"])])


def footer(canvas, doc):
    canvas.saveState()
    canvas.setStrokeColor(LINE)
    canvas.setLineWidth(0.4)
    canvas.line(20 * mm, 14 * mm, 190 * mm, 14 * mm)
    canvas.setFont("CN", 7.6)
    canvas.setFillColor(MUTED)
    canvas.drawString(20 * mm, 9.5 * mm, "CampusClaw · 身份认证与知识库问答实现方案")
    canvas.drawRightString(190 * mm, 9.5 * mm, "第 %d 页" % doc.page)
    canvas.restoreState()


def build(story):
    doc = BaseDocTemplate(OUT, pagesize=A4,
                          leftMargin=20 * mm, rightMargin=20 * mm,
                          topMargin=18 * mm, bottomMargin=18 * mm,
                          title="CampusClaw 身份认证与知识库问答实现方案",
                          author="CampusClaw")
    frame = Frame(doc.leftMargin, doc.bottomMargin, doc.width, doc.height, id="main")
    doc.addPageTemplates([PageTemplate(id="all", frames=[frame], onPage=footer)])
    doc.build(story)
    return OUT


def story():
    s = []
    # ---------------- 封面 ----------------
    s += [Spacer(1, 45 * mm)]
    s += [Paragraph("CampusClaw", S["title"])]
    s += [Spacer(1, 4 * mm)]
    s += [Paragraph("身份认证与知识库问答 · 实现方案", S["subtitle"])]
    s += [Spacer(1, 3 * mm)]
    s += [Paragraph("基于现有代码梳理：token 方案登录认证 / 知识库检索与引用标注问答",
                    ParagraphStyle("cover", parent=S["subtitle"], fontSize=10.5, leading=18))]
    s += [Spacer(1, 30 * mm)]
    s += [table(["项目", "内容"], [
        ["文档主题", "结合现有代码梳理「基于 token 的身份认证」与「基于知识库的问答」两条链路的实现方案"],
        ["代码仓库", "https://github.com/OhtoAi583/campusclaw"],
        ["对应迭代", "迭代 1（认证授权与知识库入库）、迭代 2（限班级范围检索与内容溯源）、课堂任务（token 认证、知识库问答）"],
        ["技术栈", "Go（net/http，不引入 Web 框架）+ MySQL 8.0 + React 18 + Nginx + Docker Compose；回答生成接 DeepSeek 对话网关"],
        ["验证方式", "后端单元测试 + 验收脚本 + 浏览器实测截图（截图与命令输出均取自真实运行结果）"],
    ], widths=[30 * mm, 140 * mm])]
    s += [PageBreak()]

    # ---------------- 1 概述 ----------------
    s += [Paragraph("1. 系统概览", S["h1"])]
    s += [Paragraph(
        "CampusClaw 是面向中小学教研场景的知识库底座。请求链路只有一条："
        "<b>浏览器 → Nginx → Go → MySQL</b>；浏览器与静态页面一律不可信，"
        "认证、授权、班级隔离、检索与生成全部发生在 Go 服务内。数据库只在 Compose 内部网络可达，"
        "上传目录只挂给 api 容器，不经 Nginx 静态暴露。", S["body"])]
    s += [Paragraph("本文梳理两条主线：", S["body"])]
    s += [Paragraph("① <b>基于 token 的身份认证</b>：账号口令换取 JWT 访问令牌，"
                    "受保护接口用 <font name='MONO'>Authorization: Bearer</font> 携带令牌；", S["bullet"], bulletText="•")]
    s += [Paragraph("② <b>基于知识库的问答</b>：材料入库并切分为带字符区间的切片，"
                    "提问时先在本班范围内检索，取得切片之后才调用生成模型，回答中用 [1][2] 标注出处。",
                    S["bullet"], bulletText="•")]
    s += [Spacer(1, 3 * mm)]
    s += [table(["层", "选型", "职责"], [
        ["前端", "React 18 + TypeScript + Vite", "登录页、材料页、检索页、问答演示页；只调用同源 /api"],
        ["入口", "Nginx", "托管前端构建产物；反代 /api；上传目录显式返回 404"],
        ["服务", "Go + net/http", "认证与授权、班级隔离、上传与入库、检索、回答生成"],
        ["存储", "MySQL 8.0", "用户、会话、刷新令牌、材料、知识库正文、切片与向量"],
        ["外部", "DeepSeek 对话网关", "仅在检索到切片后调用，用于生成带出处的回答"],
    ], widths=[18 * mm, 52 * mm, 100 * mm])]

    # ---------------- 2 token 认证 ----------------
    s += [PageBreak()]
    s += [Paragraph("2. 基于 token 的身份认证", S["h1"])]
    s += [Paragraph("2.1 设计目标", S["h2"])]
    s += [Paragraph(
        "token 方案与迭代 1 的服务端会话（Cookie）<b>并存</b>，两者最终换回同一组身份"
        "（user_id / role / class_id），因此授权与班级隔离的逻辑只有一份。设计时坚持三条：", S["body"])]
    s += [Paragraph("令牌载荷<b>不带权限声明</b>，role 与 class_id 每次回库读取；", S["bullet"], bulletText="1.")]
    s += [Paragraph("访问令牌短期有效，刷新令牌可轮换、可撤销；", S["bullet"], bulletText="2.")]
    s += [Paragraph("签名密钥只存在于服务端环境变量，缺失时服务拒绝启动。", S["bullet"], bulletText="3.")]

    s += [Paragraph("2.2 令牌结构", S["h2"])]
    s += [Paragraph(
        "访问令牌是 HS256 签名的紧凑 JWT，三段式 <font name='MONO'>header.payload.signature</font>。"
        "载荷刻意只保留 <font name='MONO'>sub</font>（用户 id）与 <font name='MONO'>jti</font>、"
        "<font name='MONO'>iat</font>、<font name='MONO'>exp</font>：载荷是客户端可读的，"
        "把权限声明写进去容易被误当成授权依据。", S["body"])]
    s += [code_block(code_lines("backend/internal/auth/token.go", r"type TokenClaims struct", max_lines=12),
                     "代码 2-1　令牌载荷定义（backend/internal/auth/token.go）")]

    s += [Paragraph("2.3 签发：账号口令换令牌", S["h2"])]
    s += [Paragraph(
        "登录接口先按用户名查库并做 bcrypt 校验（未知用户也会执行一次等成本的假校验，避免通过响应时间区分账号是否存在），"
        "通过后签发一对令牌：JWT 访问令牌 + 随机刷新令牌。刷新令牌只有哈希入库，因此可以随时撤销。", S["body"])]
    s += [code_block(code_lines("backend/internal/auth/token.go", r"func \(t \*TokenService\) Issue", max_lines=30),
                     "代码 2-2　签发访问令牌与刷新令牌（backend/internal/auth/token.go）")]
    s += [Paragraph(
        "接口层 <font name='MONO'>POST /api/token</font> 与 Cookie 方案共用同一套口令校验、"
        "失败文案与限流逻辑：锁定期的响应与凭据错误完全一致，不使用 429 暴露账号是否存在。", S["body"])]

    s += [Paragraph("2.4 校验：一个入口，两种凭据", S["h2"])]
    s += [Paragraph(
        "受保护接口统一经过 <font name='MONO'>RequireAuth</font>：先看 <font name='MONO'>Authorization: Bearer</font>，"
        "再退回到 Cookie 会话；两条分支最终都调用 <font name='MONO'>WithSession</font> 把同一份身份放进请求上下文。"
        "业务代码因此完全不需要知道调用方用的是哪种方案。", S["body"])]
    s += [code_block(code_lines("backend/internal/httpx/httpx.go", r"func bearerToken", max_lines=10),
                     "代码 2-3　解析 Bearer 头（backend/internal/httpx/httpx.go）")]
    s += [code_block(code_lines("backend/internal/httpx/httpx.go", r"func RequireAuth", max_lines=55),
                     "代码 2-4　统一鉴权入口：Bearer 与 Cookie 两条分支（backend/internal/httpx/httpx.go）")]
    s += [Paragraph(
        "注意其中的关键一步：校验令牌只得到 <font name='MONO'>sub</font>，随后"
        " <font name='MONO'>ByUserID</font> 回 <font name='MONO'>users</font> 表取 role 与 class_id。"
        "这正是「令牌不承载权限」的落点——即便有人拿到令牌，也无法通过改载荷提权（改了签名就不对）。", S["body"])]

    s += [Paragraph("2.5 刷新、轮换与撤销", S["h2"])]
    s += [Paragraph(
        "刷新令牌是一次性的：使用后立即删除并签发新的一对（轮换），旧刷新令牌随即失效。"
        "签名校验用 <font name='MONO'>hmac.Equal</font> 做常量时间比较，避免时序侧信道。", S["body"])]
    s += [code_block(code_lines("backend/internal/auth/token.go", r"func \(t \*TokenService\) Verify", max_lines=30),
                     "代码 2-5　校验签名与有效期（backend/internal/auth/token.go）")]
    s += [code_block(code_lines("backend/internal/db/migrations/003_refresh_tokens.sql", r"CREATE TABLE", max_lines=16),
                     "代码 2-6　刷新令牌表：只存哈希，可轮换可撤销（backend/internal/db/migrations/003_refresh_tokens.sql）")]

    s += [Paragraph("2.6 安全要点", S["h2"])]
    s += [table(["关注点", "做法", "代码位置"], [
        ["令牌被篡改", "HS256 签名校验，常量时间比较；任一字符变化即 401", "auth/token.go · Verify"],
        ["密钥泄露/缺失", "JWT_SECRET 来自环境变量，缺失时进程以非零状态退出，不落内置默认值", "config/config.go"],
        ["权限提权", "载荷不含 role/class_id，每次请求回库取身份", "httpx/httpx.go · RequireAuth"],
        ["重放与长期有效", "访问令牌 15 分钟；刷新令牌一次性轮换、可撤销", "auth/token.go · Issue/Refresh"],
        ["账号枚举", "未知用户也执行等成本假校验；失败文案与锁定期响应一致", "server/server.go · tokenLogin"],
        ["暴力破解", "按「用户名 + 客户端 IP」限流", "auth/ratelimit.go"],
        ["令牌落盘风险", "仓库不含真实密钥（.env 被 gitignore），密钥不下发浏览器", ".gitignore / .env.example"],
    ], widths=[26 * mm, 98 * mm, 46 * mm])]

    s += [Paragraph("2.7 与 Cookie 会话方案的对照", S["h2"])]
    s += [table(["维度", "服务端会话（Cookie）", "token 方案（JWT）"], [
        ["凭据形态", "不透明随机串，服务端有会话行", "自包含 JWT，服务端不存访问令牌"],
        ["浏览器是否自动携带", "是", "否，需显式加 Authorization 头"],
        ["脚本可读性", "HttpOnly，脚本读不到", "取决于存放位置；放 localStorage 则 XSS 可读"],
        ["登出能否立即失效", "能，删会话行即可", "访问令牌不能，只能等过期；刷新令牌可撤销"],
        ["服务端存储", "每个登录态一行", "仅刷新令牌一行"],
        ["适用场景", "同源浏览器站点", "开放 API、移动端、多方调用"],
    ], widths=[30 * mm, 68 * mm, 72 * mm])]

    s += [Paragraph("2.8 实测结果", S["h2"])]
    s += [table(["用例", "请求", "结果"], [
        ["换取令牌", "POST /api/token", "200，返回 access_token / refresh_token / expires_in=900"],
        ["令牌访问", "GET /api/materials，仅带 Bearer", "200，返回本班材料（未使用任何 Cookie）"],
        ["无凭据", "GET /api/materials", "401，响应体不含业务数据"],
        ["令牌被篡改", "改末位字符后 GET /api/materials", "401（签名校验失败）"],
        ["越权（学生令牌上传）", "POST /api/materials", "403，角色取自库而非令牌"],
        ["刷新轮换", "POST /api/token/refresh", "200，新旧令牌不同；旧刷新令牌再用为 401"],
        ["登出", "POST /api/token/logout", "204，刷新令牌被撤销"],
    ], widths=[34 * mm, 66 * mm, 70 * mm])]
    s += [shot(os.path.join(SHOT_TOKEN, "01-账号口令换取JWT访问令牌-展示header与payload.png"),
               "实测截图　账号口令换取 JWT：左侧展示令牌三段结构、载荷字段与过期时间，右侧为真实接口返回")]
    s += [shot(os.path.join(SHOT_TOKEN, "05-学生令牌调上传-403越权被拒.png"),
               "实测截图　用学生令牌调用教师专属上传返回 403：令牌里没有角色声明，服务端按 sub 回库取角色判定")]
    return s


def story_part2():
    s = []
    s += [PageBreak()]
    s += [Paragraph("3. 基于知识库的问答", S["h1"])]
    s += [Paragraph(
        "问答链路分两段：<b>入库侧</b>把材料正文切成带字符区间的切片并计算向量；"
        "<b>查询侧</b>先在本班范围内检索，取得切片之后才生成回答，并让每条结论都能指回原文。", S["body"])]

    s += [Paragraph("3.1 总体链路", S["h2"])]
    s += [code_block(
        "入库侧：上传 → 校验（白名单/大小/UTF-8）→ 落盘 → 同一事务写入\n"
        "        materials + knowledge_entries + kb_chunks（切片 + 向量）\n"
        "\n"
        "查询侧：提问（只用最新这一句）\n"
        "        → 本班检索（class_id 取自会话）\n"
        "        → 有切片？ 否 → 返回「资料中未找到相关内容」，不调用生成模型\n"
        "                  是 → 组装（材料标题 + 切片序号 + 切片正文 + 编号）\n"
        "                       → 生成模型 → 回答中用 [1][2] 标注 → 返回 citations",
        "代码 3-1　两条链路总览（示意）")]

    s += [Paragraph("3.2 入库：一次事务写三张表", S["h2"])]
    s += [Paragraph(
        "材料、正文与切片必须同生共死：只写材料会让检索拿不到数据，只写切片则会让下载指向不存在的文件。"
        "因此切片与向量在同一事务内写入，失败整体回滚并删除已落盘的文件。", S["body"])]
    s += [code_block(code_lines("backend/internal/kb/index.go", r"func \(ix \*Indexer\) WriteTx", max_lines=28),
                     "代码 3-2　切片与向量写入事务（backend/internal/kb/index.go）")]

    s += [Paragraph("3.3 分块与溯源", S["h2"])]
    s += [Paragraph(
        "切片是检索与溯源的最小单位。分块策略是「Markdown 标题优先断开，过长的段落再按定长窗口切分并保留重叠」，"
        "每个切片都记录它在正文中的<b>字符区间</b>——这是后面能核对出处的前提。", S["body"])]
    s += [code_block(code_lines("backend/internal/kb/chunk.go", r"type Chunk struct", max_lines=8),
                     "代码 3-3　切片结构：文本 + 序号 + 字符区间（backend/internal/kb/chunk.go）")]
    s += [Paragraph(
        "开发中在这里发现过一个真实缺陷：早期「合并过短段落」的逻辑会直接<b>跳过</b>两个标题之间的正文，"
        "导致长文档只索引到开头的一小段（表现为检索永远命中开头）。修复为「延长上一段的结束位置」，"
        "并补了回归测试 <font name='MONO'>TestSplitKeepsContentWithDenseHeadings</font>，断言块区间的并集覆盖全文。", S["body"])]

    s += [Paragraph("3.4 向量化与检索", S["h2"])]
    s += [Paragraph(
        "每个切片计算一个定长向量（字符 n-gram 经哈希技巧映射 + L2 归一化），检索用余弦相似度排序。"
        "由于向量已归一化，余弦退化为点积，计算足够快；同一输入必得同一向量，因此排序可复现、可判定。", S["body"])]
    s += [code_block(code_lines("backend/internal/kb/embed.go", r"func \(h \*HashingEmbedder\) Embed", max_lines=20),
                     "代码 3-4　确定性嵌入（backend/internal/kb/embed.go）")]
    s += [Paragraph(
        "<b>班级隔离有两条防线</b>：候选集在 SQL 层就按会话班级过滤（跨班数据从不进入候选集），"
        "应用层再做一次断言。即便有人改坏了 SQL 条件，跨班切片也不会被返回。", S["body"])]
    s += [code_block(code_lines("backend/internal/search/search.go", r"func \(s \*Service\) Search", max_lines=22),
                     "代码 3-5　检索入口：班级只来自会话（backend/internal/search/search.go）")]
    s += [Paragraph(
        "低于阈值的候选按「没有匹配」处理：哈希嵌入会有少量碰撞噪声，不设阈值时无关提问也会返回一堆低分结果。"
        "跨班内容既不进入候选集、也不体现在结果条数与错误信息中，对外表现为 200 + 空列表（不是 403/404，"
        "避免暗示「该资料属于其他班级」）。", S["body"])]

    s += [Paragraph("3.5 先检索，再生成", S["h2"])]
    s += [Paragraph(
        "问答服务只做两件事：调用检索拿到切片；有切片才调用生成模型。"
        "<b>没有命中切片时完全不进入生成环节</b>——避免模型凭参数记忆硬答天气、比分一类问题。", S["body"])]
    s += [code_block(code_lines("backend/internal/answer/answer.go", r"func \(s \*Service\) Ask", max_lines=26),
                     "代码 3-6　先检索再生成；无命中直接返回固定文案（backend/internal/answer/answer.go）")]
    s += [Paragraph("交给模型的内容由服务端组装，客户端注入不了 system 消息：", S["body"])]
    s += [code_block(code_lines("backend/internal/answer/answer.go", r"func BuildPrompt", max_lines=18),
                     "代码 3-7　提示词组装：只有标题、切片序号、切片正文与提问（backend/internal/answer/answer.go）")]

    s += [Paragraph("3.6 引用标注与出处", S["h2"])]
    s += [Paragraph(
        "返回体里的 <font name='MONO'>citations[i].index</font> 与回答中的 <font name='MONO'>[i]</font> 严格对应，"
        "每条出处包含材料 id、标题、原始文件名、切片序号、字符区间与摘录。"
        "前端点击出处可回到材料详情并高亮该字符区间；摘录取自 MySQL 的切片正文，而不是向量库的载荷。", S["body"])]
    s += [table(["字段", "含义", "用途"], [
        ["material_id / material_title", "来源材料", "点击可打开详情（仍受班级隔离约束）"],
        ["chunk_index", "切片序号", "定位到第几块"],
        ["start_offset / end_offset", "在正文中的字符区间", "按区间截取正文即得到片段，可逐字符核对"],
        ["snippet", "切片正文摘录", "直接展示依据，无需二次请求"],
        ["score", "相似度", "用于展示相关度；低于阈值的候选不返回"],
    ], widths=[46 * mm, 44 * mm, 80 * mm])]

    s += [Paragraph("3.7 生成模型接入", S["h2"])]
    s += [Paragraph(
        "生成这一步做成接口 <font name='MONO'>ChatClient</font>：配置了网关密钥就用真实模型（DeepSeek，"
        "兼容 OpenAI 的 /chat/completions），未配置则退回本地摘录实现（内容全部来自检索结果，同样可溯源），"
        "便于离线复现。返回体的 <font name='MONO'>engine</font> 会标明本次由谁生成。", S["body"])]
    s += [code_block(code_lines("backend/internal/answer/gateway.go", r"func \(g \*GatewayClient\) Answer", max_lines=34),
                     "代码 3-8　调用对话网关（backend/internal/answer/gateway.go）")]

    s += [Paragraph("3.8 实测结果", S["h2"])]
    s += [table(["用例", "结果"], [
        ["问本班材料里有依据的问题", "200，answer 带 [1][2] 标注，citations 2 条，engine=gateway:deepseek-chat"],
        ["问本班材料没有依据的问题（天气）", "200，answer=「资料中未找到相关内容」，citations 为空，model_called=false"],
        ["未登录提问", "401，不返回任何回答或切片"],
        ["跨班提问", "200 + 空结果（不暗示资料归属）"],
        ["切片区间一致性", "按 start/end 截取正文与返回片段逐字符一致"],
    ], widths=[62 * mm, 108 * mm])]
    s += [shot(os.path.join(SHOT_ASK, "04-真实模型DeepSeek回答-带1标注与出处.png"),
               "实测截图　真实模型回答：每条结论后用 [1][2] 标注，下方列出出处（材料标题、块序号、字符区间、摘录）")]
    s += [shot(os.path.join(SHOT_ASK, "05-天气类问题-无依据不调用模型.png"),
               "实测截图　问「今天上海天气怎么样」：本班无依据 → 不调用生成模型，返回固定文案，citations 为空")]

    # ---------------- 4 衔接 ----------------
    s += [PageBreak()]
    s += [Paragraph("4. 两条链路的衔接", S["h1"])]
    s += [Paragraph(
        "认证与问答并不是两套独立系统，它们在同一条链路上前后相接："
        "<b>认证决定你是谁、属于哪个班；问答用这个班级限定检索范围；"
        "检索结果又通过材料 id 回到同一个受鉴权保护的详情接口</b>。", S["body"])]
    s += [code_block(
        "账号口令 ──► 访问令牌(JWT) ─┐\n"
        "                            ├─► RequireAuth ─► 会话身份(user_id, role, class_id)\n"
        "Cookie 会话 ────────────────┘                        │\n"
        "                                                    ▼\n"
        "                                        WHERE class_id = 会话班级\n"
        "                                                    │\n"
        "                     ┌──────────────────────────────┴──────────────────┐\n"
        "                     ▼                                                 ▼\n"
        "              检索 /api/search（切片 + 分数）              问答 /api/ask（前 4 条切片）\n"
        "                     │                                                 │\n"
        "                     └────────────► citations ◄──────────────────────┘\n"
        "                                        │\n"
        "                                        ▼\n"
        "                     点击出处 → GET /api/materials/{id}（同样受班级隔离保护）",
        "代码 4-1　认证 → 隔离 → 检索 → 引用 的衔接关系")]

    # ---------------- 5 限制 ----------------
    s += [Paragraph("5. 已知限制与后续", S["h1"])]
    s += [table(["限制", "影响", "后续做法"], [
        ["访问令牌无状态", "登出后令牌在有效期内（15 分钟）仍可用", "需要立即失效时引入黑名单或缩短有效期"],
        ["嵌入是词形相似", "同义改写、跨语言检索会明显变弱", "把 Embedder 换成真实语义模型（接口已预留）"],
        ["单实例部署", "登录限流在进程内存中，多副本会不一致", "限流状态外置（Redis）后再扩容"],
        ["知识库只支持纯文本", "PDF / Word / 图片无法入库", "后续迭代增加解析与 OCR"],
        ["未实现流式输出与多轮上下文", "回答一次返回，不保留对话历史", "按课程安排在第 5 课实现"],
        ["向量存于 MySQL", "数据量增大后相似度计算会变慢", "数据量上来后迁移到向量库（Qdrant 等）"],
    ], widths=[36 * mm, 62 * mm, 72 * mm])]

    # ---------------- 附录 ----------------
    s += [PageBreak()]
    s += [Paragraph("附录 A　代码索引", S["h1"])]
    s += [table(["模块", "文件", "关键函数"], [
        ["令牌签发与校验", "backend/internal/auth/token.go", "Issue / Verify / Refresh / Revoke"],
        ["会话存储", "backend/internal/auth/session.go", "Create / Lookup / ByUserID"],
        ["登录限流", "backend/internal/auth/ratelimit.go", "Allowed / Fail / Reset"],
        ["统一鉴权入口", "backend/internal/httpx/httpx.go", "RequireAuth / bearerToken / SetSessionCookie"],
        ["token 接口", "backend/internal/server/server.go", "tokenLogin / tokenRefresh / tokenLogout"],
        ["分块与向量", "backend/internal/kb/chunk.go · embed.go · index.go", "Split / Embed / WriteTx / RebuildAll"],
        ["检索", "backend/internal/search/search.go", "Search（班级过滤 + 相似度排序）"],
        ["问答", "backend/internal/answer/answer.go · gateway.go", "Ask / BuildPrompt / GatewayClient.Answer"],
        ["入库事务", "backend/internal/materials/store.go", "Create（三表同事务）"],
        ["数据模型", "backend/internal/db/migrations/*.sql", "users / sessions / refresh_tokens / materials / knowledge_entries / kb_chunks"],
    ], widths=[34 * mm, 76 * mm, 60 * mm])]

    s += [Paragraph("附录 B　复现步骤", S["h1"])]
    s += [code_block(
        "# 1) 启动（Docker Compose，唯一标准启动方式）\n"
        "cp .env.example .env      # 填 SESSION_SECRET、DB_*、SEED_*_PASSWORD、JWT_SECRET\n"
        "docker compose up --build -d\n"
        "# 打开 http://localhost:8080\n"
        "\n"
        "# 2) token 方案\n"
        "curl -s -X POST localhost:8080/api/token -H 'Content-Type: application/json' \\\n"
        "     -d '{\"username\":\"student_a1\",\"password\":\"<预置口令>\"}'\n"
        "curl -s -H \"Authorization: Bearer <access_token>\" localhost:8080/api/materials\n"
        "\n"
        "# 3) 知识库问答\n"
        "curl -s -X POST localhost:8080/api/ask -H 'Content-Type: application/json' \\\n"
        "     -H 'Cookie: campusclaw_session=<会话>' \\\n"
        "     -d '{\"question\":\"导入环节为什么要用情境图？\"}'\n"
        "\n"
        "# 4) 验收脚本（逐条对照规约）\n"
        "BASE_URL=http://localhost:8080 TEACHER_PW=... STUDENT_A_PW=... STUDENT_B_PW=... ./scripts/verify.sh\n"
        "./scripts/verify-retrieval.sh\n"
        "./scripts/eval-retrieval.sh        # 评测集：命中率 + 隔离用例",
        "附录 B　从零复现与验证")]

    s += [Spacer(1, 6 * mm)]
    s += [Paragraph(
        "本文所有代码片段均从仓库源码按函数抽取，实测结果与截图取自本机真实运行（Nginx → Go → MySQL，"
        "回答生成接入 DeepSeek 网关）。仓库地址：https://github.com/OhtoAi583/campusclaw", S["caption"])]
    return s


def main():
    out = build(story() + story_part2())
    print("已生成:", out)


if __name__ == "__main__":
    main()
