#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""把《小组项目选题说明.md》渲染为 PDF（reportlab）。

Markdown 是唯一内容源；只用到一个很小的子集：标题、段落、粗体、行内代码、
无序/有序列表、表格、代码块、分隔线。
"""
import os
import re

from reportlab.lib import colors
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.lib.units import mm
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (BaseDocTemplate, Frame, KeepTogether, PageBreak, PageTemplate,
                                Paragraph, Preformatted, Spacer, Table, TableStyle)

HERE = os.path.dirname(os.path.abspath(__file__))
MD = os.path.join(HERE, "小组项目选题说明.md")
PDF = os.path.join(HERE, "小组项目选题说明.pdf")

BRAND = colors.HexColor("#9c1f24")
INK = colors.HexColor("#1f1c1b")
MUTED = colors.HexColor("#6f6a67")
LINE = colors.HexColor("#e3ddd7")
CODE_BG = colors.HexColor("#f5f3f1")

pdfmetrics.registerFont(TTFont("CN", "/System/Library/Fonts/STHeiti Light.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("CNB", "/System/Library/Fonts/STHeiti Medium.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("MONO", "/System/Library/Fonts/Menlo.ttc", subfontIndex=0))

STYLES = {
    "h1": ParagraphStyle("h1", fontName="CNB", fontSize=19, leading=27, textColor=BRAND, spaceAfter=8),
    "h2": ParagraphStyle("h2", fontName="CNB", fontSize=13, leading=21, textColor=BRAND, spaceBefore=13, spaceAfter=5),
    "h3": ParagraphStyle("h3", fontName="CNB", fontSize=11, leading=18, textColor=INK, spaceBefore=9, spaceAfter=3),
    "body": ParagraphStyle("body", fontName="CN", fontSize=9.6, leading=16, textColor=INK, spaceAfter=4),
    "li": ParagraphStyle("li", fontName="CN", fontSize=9.6, leading=16, textColor=INK,
                         leftIndent=12, bulletIndent=3, spaceAfter=2),
    "th": ParagraphStyle("th", fontName="CNB", fontSize=9.2, leading=13.5, textColor=colors.white),
    "td": ParagraphStyle("td", fontName="CN", fontSize=9.2, leading=13.5, textColor=INK),
    "note": ParagraphStyle("note", fontName="CN", fontSize=8.6, leading=13, textColor=MUTED, spaceBefore=3),
}


def esc(t):
    return t.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")


def inline(text):
    text = esc(text)
    text = re.sub(r"\*\*(.+?)\*\*", r"<b>\1</b>", text)
    text = re.sub(r"`(.+?)`", r"<font name='MONO' size=8.8>\1</font>", text)
    return text


def code_block(text):
    mono = "MONO" if all(ord(c) < 128 for c in text) else "CN"
    wrapped = []
    for raw in text.split("\n"):
        line = raw.replace("\t", "    ")
        limit = 112 if mono == "MONO" else 104
        while len(line) > limit:
            wrapped.append(line[:limit])
            line = "    " + line[limit:]
        wrapped.append(line)
    body = Preformatted("\n".join(wrapped),
                        ParagraphStyle("code", fontName=mono, fontSize=8, leading=11.5, textColor=INK))
    t = Table([[body]], colWidths=[170 * mm])
    t.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, -1), CODE_BG),
                           ("BOX", (0, 0), (-1, -1), 0.5, LINE),
                           ("LEFTPADDING", (0, 0), (-1, -1), 7), ("RIGHTPADDING", (0, 0), (-1, -1), 7),
                           ("TOPPADDING", (0, 0), (-1, -1), 6), ("BOTTOMPADDING", (0, 0), (-1, -1), 6)]))
    return t


def make_table(rows, total=170 * mm):
    ncols = max(len(r) for r in rows)
    if ncols == 2:
        widths = [42 * mm, total - 42 * mm]
    elif ncols == 3:
        widths = [22 * mm, 34 * mm, total - 56 * mm]
    elif ncols == 4:
        widths = [16 * mm, 52 * mm, 16 * mm, total - 84 * mm]
    else:
        widths = [total / ncols] * ncols
    data = [[Paragraph(inline(c), STYLES["th"]) for c in rows[0]]]
    for r in rows[1:]:
        data.append([Paragraph(inline(c), STYLES["td"]) for c in r])
    t = Table(data, colWidths=widths, repeatRows=1)
    t.setStyle(TableStyle([("BACKGROUND", (0, 0), (-1, 0), BRAND),
                           ("ROWBACKGROUNDS", (0, 1), (-1, -1), [colors.white, colors.HexColor("#faf8f6")]),
                           ("GRID", (0, 0), (-1, -1), 0.4, LINE),
                           ("VALIGN", (0, 0), (-1, -1), "TOP"),
                           ("LEFTPADDING", (0, 0), (-1, -1), 5), ("RIGHTPADDING", (0, 0), (-1, -1), 5),
                           ("TOPPADDING", (0, 0), (-1, -1), 3.5), ("BOTTOMPADDING", (0, 0), (-1, -1), 3.5)]))
    return t


def parse(md_text):
    story, lines, i = [], md_text.split("\n"), 0
    while i < len(lines):
        line = lines[i]
        if line.startswith("```"):
            buf, i = [], i + 1
            while i < len(lines) and not lines[i].startswith("```"):
                buf.append(lines[i]); i += 1
            i += 1
            story += [code_block("\n".join(buf)), Spacer(1, 5)]
            continue
        if re.match(r"^\|.*\|$", line):
            rows = []
            while i < len(lines) and re.match(r"^\|.*\|$", lines[i]):
                cells = [c.strip() for c in lines[i].strip().strip("|").split("|")]
                if not all(re.fullmatch(r":?-{2,}:?", c) for c in cells):
                    rows.append(cells)
                i += 1
            story += [make_table(rows), Spacer(1, 6)]
            continue
        if line.startswith("### "):
            story.append(Paragraph(inline(line[4:]), STYLES["h3"]))
        elif line.startswith("## "):
            story.append(Paragraph(inline(line[3:]), STYLES["h2"]))
        elif line.startswith("# "):
            story.append(Paragraph(inline(line[2:]), STYLES["h1"]))
        elif line.strip() == "---":
            story.append(Spacer(1, 4 * mm))
        elif re.match(r"^-\s+", line):
            while i < len(lines) and re.match(r"^-\s+", lines[i]):
                story.append(Paragraph(inline(re.sub(r"^-\s+", "", lines[i])), STYLES["li"], bulletText="•"))
                i += 1
            story.append(Spacer(1, 3))
            continue
        elif re.match(r"^\d+\.\s+", line):
            n = 1
            while i < len(lines) and re.match(r"^\d+\.\s+", lines[i]):
                story.append(Paragraph(inline(re.sub(r"^\d+\.\s+", "", lines[i])),
                                       STYLES["li"], bulletText="%d." % n))
                n += 1; i += 1
            story.append(Spacer(1, 3))
            continue
        elif line.strip():
            story.append(Paragraph(inline(line), STYLES["body"]))
        i += 1
    return story


def footer(canvas, doc):
    canvas.saveState()
    canvas.setStrokeColor(LINE); canvas.setLineWidth(0.4)
    canvas.line(20 * mm, 14 * mm, 190 * mm, 14 * mm)
    canvas.setFont("CN", 7.6); canvas.setFillColor(MUTED)
    canvas.drawString(20 * mm, 9.5 * mm, "CampusClaw 小组项目选题说明 · 龙浩")
    canvas.drawRightString(190 * mm, 9.5 * mm, "第 %d 页" % doc.page)
    canvas.restoreState()


def main():
    doc = BaseDocTemplate(PDF, pagesize=A4, leftMargin=20 * mm, rightMargin=20 * mm,
                          topMargin=16 * mm, bottomMargin=18 * mm,
                          title="CampusClaw 小组项目选题说明", author="龙浩")
    frame = Frame(doc.leftMargin, doc.bottomMargin, doc.width, doc.height, id="main")
    doc.addPageTemplates([PageTemplate(id="all", frames=[frame], onPage=footer)])
    md_text = open(MD, encoding="utf-8").read()
    doc.build(parse(md_text))
    print("已生成:", PDF)


if __name__ == "__main__":
    main()
