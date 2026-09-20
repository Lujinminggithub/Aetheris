from __future__ import annotations

import os
import math
from pathlib import Path
from typing import Iterable, Sequence

from PIL import Image, ImageDraw, ImageFont
from docx import Document
from docx.enum.section import WD_SECTION
from docx.enum.style import WD_STYLE_TYPE
from docx.enum.table import WD_CELL_VERTICAL_ALIGNMENT, WD_TABLE_ALIGNMENT
from docx.enum.text import WD_ALIGN_PARAGRAPH, WD_BREAK
from docx.oxml import OxmlElement
from docx.oxml.ns import qn
from docx.shared import Cm, Inches, Pt, RGBColor


ROOT = Path(__file__).resolve().parents[1]
OUTPUT = ROOT / "docs" / "Aetheris产品设计与实现说明书.docx"
ASSET_DIR = ROOT / "build" / "product-design-assets"

TEAL = "176B68"
DARK = "203033"
GREEN = "3B8C6E"
AMBER = "C68A2B"
BLUE = "356A8A"
LIGHT_TEAL = "E7F2F0"
LIGHT_BLUE = "EAF1F6"
LIGHT_AMBER = "F8F0E2"
LIGHT_GRAY = "F2F4F4"
MID_GRAY = "D9E0E0"
TEXT_GRAY = "526063"
WHITE = "FFFFFF"


def set_cell_shading(cell, fill: str) -> None:
    tc_pr = cell._tc.get_or_add_tcPr()
    shd = tc_pr.find(qn("w:shd"))
    if shd is None:
        shd = OxmlElement("w:shd")
        tc_pr.append(shd)
    shd.set(qn("w:fill"), fill)


def set_cell_border(cell, color: str = MID_GRAY, size: str = "4") -> None:
    tc_pr = cell._tc.get_or_add_tcPr()
    borders = tc_pr.first_child_found_in("w:tcBorders")
    if borders is None:
        borders = OxmlElement("w:tcBorders")
        tc_pr.append(borders)
    for edge in ("top", "left", "bottom", "right", "insideH", "insideV"):
        tag = "w:" + edge
        element = borders.find(qn(tag))
        if element is None:
            element = OxmlElement(tag)
            borders.append(element)
        element.set(qn("w:val"), "single")
        element.set(qn("w:sz"), size)
        element.set(qn("w:color"), color)


def set_repeat_table_header(row) -> None:
    tr_pr = row._tr.get_or_add_trPr()
    tbl_header = OxmlElement("w:tblHeader")
    tbl_header.set(qn("w:val"), "true")
    tr_pr.append(tbl_header)


def set_keep_with_next(paragraph, value: bool = True) -> None:
    p_pr = paragraph._p.get_or_add_pPr()
    keep = p_pr.find(qn("w:keepNext"))
    if value and keep is None:
        keep = OxmlElement("w:keepNext")
        p_pr.append(keep)
    elif not value and keep is not None:
        p_pr.remove(keep)


def set_keep_together(paragraph, value: bool = True) -> None:
    p_pr = paragraph._p.get_or_add_pPr()
    keep = p_pr.find(qn("w:keepLines"))
    if value and keep is None:
        keep = OxmlElement("w:keepLines")
        p_pr.append(keep)


def add_page_number(paragraph) -> None:
    paragraph.alignment = WD_ALIGN_PARAGRAPH.CENTER
    run = paragraph.add_run()
    begin = OxmlElement("w:fldChar")
    begin.set(qn("w:fldCharType"), "begin")
    instr = OxmlElement("w:instrText")
    instr.set(qn("xml:space"), "preserve")
    instr.text = " PAGE "
    separate = OxmlElement("w:fldChar")
    separate.set(qn("w:fldCharType"), "separate")
    text = OxmlElement("w:t")
    text.text = "1"
    end = OxmlElement("w:fldChar")
    end.set(qn("w:fldCharType"), "end")
    run._r.extend([begin, instr, separate, text, end])


def set_east_asia_font(run, font_name: str) -> None:
    run.font.name = font_name
    run._element.rPr.rFonts.set(qn("w:eastAsia"), font_name)


def add_text(paragraph, text: str, *, bold: bool = False, color: str | None = None, size: float | None = None, font: str = "微软雅黑"):
    run = paragraph.add_run(text)
    run.bold = bold
    set_east_asia_font(run, font)
    if color:
        run.font.color.rgb = RGBColor.from_string(color)
    if size:
        run.font.size = Pt(size)
    return run


def add_bullet(doc: Document, text: str, level: int = 0) -> None:
    style = "List Bullet" if level == 0 else "List Bullet 2"
    p = doc.add_paragraph(style=style)
    add_text(p, text)
    p.paragraph_format.space_after = Pt(2)


def add_number(doc: Document, text: str) -> None:
    p = doc.add_paragraph(style="List Number")
    add_text(p, text)
    p.paragraph_format.space_after = Pt(2)


def add_callout(doc: Document, title: str, body: str, kind: str = "info") -> None:
    colors = {
        "info": (LIGHT_BLUE, BLUE),
        "success": (LIGHT_TEAL, TEAL),
        "warning": (LIGHT_AMBER, AMBER),
        "neutral": (LIGHT_GRAY, TEXT_GRAY),
    }
    fill, accent = colors[kind]
    table = doc.add_table(rows=1, cols=2)
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    table.autofit = False
    table.columns[0].width = Cm(0.22)
    table.columns[1].width = Cm(15.8)
    left, right = table.rows[0].cells
    set_cell_shading(left, accent)
    set_cell_shading(right, fill)
    for cell in (left, right):
        cell.vertical_alignment = WD_CELL_VERTICAL_ALIGNMENT.CENTER
        set_cell_border(cell, fill, "0")
    p = right.paragraphs[0]
    p.paragraph_format.space_after = Pt(2)
    add_text(p, title, bold=True, color=accent)
    p2 = right.add_paragraph()
    p2.paragraph_format.space_after = Pt(0)
    add_text(p2, body, color=DARK)
    doc.add_paragraph().paragraph_format.space_after = Pt(0)


def add_table(doc: Document, headers: Sequence[str], rows: Iterable[Sequence[str]], widths: Sequence[float] | None = None, font_size: float = 8.3):
    rows = list(rows)
    table = doc.add_table(rows=1, cols=len(headers))
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    table.autofit = widths is None
    header = table.rows[0]
    set_repeat_table_header(header)
    for index, value in enumerate(headers):
        cell = header.cells[index]
        set_cell_shading(cell, TEAL)
        set_cell_border(cell, WHITE, "3")
        cell.vertical_alignment = WD_CELL_VERTICAL_ALIGNMENT.CENTER
        p = cell.paragraphs[0]
        p.alignment = WD_ALIGN_PARAGRAPH.CENTER
        p.paragraph_format.space_after = Pt(0)
        add_text(p, value, bold=True, color=WHITE, size=font_size)
    for row_index, values in enumerate(rows):
        cells = table.add_row().cells
        for col_index, value in enumerate(values):
            cell = cells[col_index]
            if row_index % 2:
                set_cell_shading(cell, "F8FAFA")
            set_cell_border(cell)
            cell.vertical_alignment = WD_CELL_VERTICAL_ALIGNMENT.CENTER
            p = cell.paragraphs[0]
            p.paragraph_format.space_after = Pt(0)
            add_text(p, str(value), size=font_size)
    if widths:
        for row in table.rows:
            for index, width in enumerate(widths):
                row.cells[index].width = Cm(width)
    doc.add_paragraph().paragraph_format.space_after = Pt(0)
    return table


def add_code(doc: Document, lines: str) -> None:
    table = doc.add_table(rows=1, cols=1)
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    cell = table.cell(0, 0)
    set_cell_shading(cell, "F5F7F7")
    set_cell_border(cell, "CBD3D3", "4")
    p = cell.paragraphs[0]
    p.paragraph_format.space_after = Pt(0)
    p.paragraph_format.line_spacing = 1.0
    for i, line in enumerate(lines.strip("\n").splitlines()):
        if i:
            p.add_run().add_break()
        add_text(p, line, size=8.0, color=DARK, font="Consolas")
    doc.add_paragraph().paragraph_format.space_after = Pt(0)


def add_status_strip(doc: Document, owner: str, runtime: str, status: str, interfaces: str) -> None:
    add_table(
        doc,
        ["责任边界", "运行形态", "当前状态", "主要接口"],
        [[owner, runtime, status, interfaces]],
        widths=[3.6, 3.5, 3.7, 5.4],
        font_size=8.0,
    )


def chapter(doc: Document, number: int, title: str, purpose: str) -> None:
    if len(doc.paragraphs) > 5:
        doc.add_page_break()
    p = doc.add_heading(f"第 {number} 章  {title}", level=1)
    set_keep_with_next(p)
    add_callout(doc, "本章目标", purpose, "success")


def section(doc: Document, title: str, level: int = 2) -> None:
    p = doc.add_heading(title, level=level)
    set_keep_with_next(p)


def paragraph(doc: Document, text: str, *, bold_prefix: str | None = None) -> None:
    p = doc.add_paragraph()
    p.paragraph_format.space_after = Pt(5)
    p.paragraph_format.line_spacing = 1.25
    set_keep_together(p)
    if bold_prefix and text.startswith(bold_prefix):
        add_text(p, bold_prefix, bold=True, color=DARK)
        add_text(p, text[len(bold_prefix):])
    else:
        add_text(p, text)


class DiagramCanvas:
    def __init__(self, width: int = 2048, height: int = 1152):
        self.width = width
        self.height = height
        self.image = Image.new("RGB", (width, height), "white")
        self.draw = ImageDraw.Draw(self.image)
        self.regular_font = Path(r"C:\Windows\Fonts\msyh.ttc")
        self.bold_font = Path(r"C:\Windows\Fonts\msyhbd.ttc")
        if not self.regular_font.exists():
            self.regular_font = Path(r"C:\Windows\Fonts\arial.ttf")
        if not self.bold_font.exists():
            self.bold_font = self.regular_font

    def point(self, x: float, y: float) -> tuple[int, int]:
        return int(x * self.width), int((1 - y) * self.height)

    def font(self, size: float, bold: bool = False):
        path = self.bold_font if bold else self.regular_font
        return ImageFont.truetype(str(path), max(10, int(size * self.height / 300)))

    def text(self, x: float, y: float, value: str, *, ha: str = "center", va: str = "center", fontsize: float = 10, fontweight: str | None = None, color: str = "#203033", wrap: bool = False):
        del wrap
        font = self.font(fontsize, fontweight == "bold")
        px, py = self.point(x, y)
        bbox = self.draw.multiline_textbbox((0, 0), value, font=font, align=ha, spacing=5)
        w = bbox[2] - bbox[0]
        h = bbox[3] - bbox[1]
        tx = px - w / 2 if ha == "center" else px
        if va == "top":
            ty = py
        elif va == "bottom":
            ty = py - h
        else:
            ty = py - h / 2
        self.draw.multiline_text((tx, ty), value, font=font, fill=color, align=ha, spacing=5)


def box(ax: DiagramCanvas, xy, width, height, title, subtitle, color=TEAL, fill=LIGHT_TEAL, title_size=12, body_size=9):
    x, y = xy
    left, top = ax.point(x, y + height)
    right, bottom = ax.point(x + width, y)
    ax.draw.rounded_rectangle((left, top, right, bottom), radius=22, fill="#" + fill, outline="#" + color, width=3)
    ax.text(x + width / 2, y + height * 0.65, title, ha="center", va="center", fontsize=title_size, fontweight="bold", color="#" + DARK)
    ax.text(x + width / 2, y + height * 0.31, subtitle, ha="center", va="center", fontsize=body_size, color="#" + TEXT_GRAY, wrap=True)


def arrow(ax: DiagramCanvas, start, end, color=TEXT_GRAY, label: str | None = None):
    x1, y1 = ax.point(*start)
    x2, y2 = ax.point(*end)
    fill = "#" + color
    ax.draw.line((x1, y1, x2, y2), fill=fill, width=4)
    angle = math.atan2(y2 - y1, x2 - x1)
    length = 15
    spread = 0.55
    p1 = (x2 - length * math.cos(angle - spread), y2 - length * math.sin(angle - spread))
    p2 = (x2 - length * math.cos(angle + spread), y2 - length * math.sin(angle + spread))
    ax.draw.polygon([(x2, y2), p1, p2], fill=fill)
    if label:
        ax.text((start[0] + end[0]) / 2, (start[1] + end[1]) / 2 + 0.025, label, ha="center", va="center", fontsize=8, color="#" + color)


def save_diagram(name: str, draw) -> Path:
    ASSET_DIR.mkdir(parents=True, exist_ok=True)
    path = ASSET_DIR / name
    canvas = DiagramCanvas()
    draw(canvas)
    canvas.image.save(path, optimize=True)
    return path


def build_diagrams() -> dict[str, Path]:
    def overall(ax):
        ax.text(0.5, 0.965, "Aetheris 总体运行架构", ha="center", va="top", fontsize=18, fontweight="bold", color="#" + DARK)
        box(ax, (0.04, 0.62), 0.22, 0.22, "Windows 终端", "Setup · Service · Core\nLocal Lens · VS Code 扩展", TEAL, LIGHT_TEAL)
        box(ax, (0.38, 0.62), 0.24, 0.22, "Go Server", "设备 API · Admin API\n清洗 / 效能 / Episode / RAG Worker", BLUE, LIGHT_BLUE)
        box(ax, (0.74, 0.62), 0.22, 0.22, "Admin Web", "React + TypeScript + Vite\n登录、管理、分析与查询", AMBER, LIGHT_AMBER)
        box(ax, (0.18, 0.17), 0.22, 0.22, "PostgreSQL", "身份、原始事件、事实、任务\n引用、审计与派生数据", GREEN, LIGHT_TEAL)
        box(ax, (0.46, 0.17), 0.18, 0.22, "Qdrant", "768 维活动向量\n仅保存过滤维度", BLUE, LIGHT_BLUE)
        box(ax, (0.70, 0.17), 0.25, 0.22, "Model Gateway", "Python 适配层\nOllama 默认 · Dify 可选", TEAL, LIGHT_TEAL)
        arrow(ax, (0.26, 0.73), (0.38, 0.73), label="HTTPS / Device Token")
        arrow(ax, (0.74, 0.73), (0.62, 0.73), label="Cookie / CSRF")
        arrow(ax, (0.47, 0.62), (0.34, 0.39), label="SQL")
        arrow(ax, (0.55, 0.62), (0.55, 0.39), label="向量接口")
        arrow(ax, (0.61, 0.62), (0.77, 0.39), label="内部 HTTP")
        ax.text(0.5, 0.06, "模块之间通过版本化 API、事件合同和数据投影连接；任何模型组件都不直接访问终端或数据库。", ha="center", fontsize=9, color="#" + TEXT_GRAY)

    def client(ax):
        ax.text(0.5, 0.965, "Windows 客户端组件与权限边界", ha="center", va="top", fontsize=18, fontweight="bold", color="#" + DARK)
        box(ax, (0.05, 0.64), 0.22, 0.20, "Aetheris Core Service", "LocalSystem · 延迟自动启动\n只监管会话和进程", BLUE, LIGHT_BLUE)
        box(ax, (0.38, 0.64), 0.24, 0.20, "Aetheris Core", "当前用户 · 托盘\n适配器 / 脱敏 / 队列 / 上传", TEAL, LIGHT_TEAL)
        box(ax, (0.73, 0.64), 0.22, 0.20, "Local Lens", "127.0.0.1:15473\n状态、授权、项目与证据", AMBER, LIGHT_AMBER)
        box(ax, (0.05, 0.18), 0.22, 0.20, "VS Code 扩展", "编辑元数据聚合\n不保存代码正文", GREEN, LIGHT_TEAL)
        box(ax, (0.38, 0.18), 0.24, 0.20, "本地安全存储", "DPAPI 凭据 · SQLite 队列\n项目注册表 · 授权决定", BLUE, LIGHT_BLUE)
        box(ax, (0.73, 0.18), 0.22, 0.20, "远端 Go Server", "Heartbeat · Policy · Projects\nAdapter Health · Ingest", TEAL, LIGHT_TEAL)
        arrow(ax, (0.27, 0.74), (0.38, 0.74), label="受保护 IPC")
        arrow(ax, (0.62, 0.74), (0.73, 0.74), label="回环控制")
        arrow(ax, (0.27, 0.28), (0.38, 0.28), label="命名管道")
        arrow(ax, (0.50, 0.64), (0.50, 0.38), label="本地读写")
        arrow(ax, (0.62, 0.28), (0.73, 0.28), label="仅出站 HTTPS")
        ax.text(0.5, 0.07, "Service 不读取事件、截图、设备 Token 或 SQLite；采集与 DPAPI 始终运行在交互用户身份下。", ha="center", fontsize=9, color="#" + TEXT_GRAY)

    def pipeline(ax):
        ax.text(0.5, 0.965, "从本地信号到可查询事实的数据链路", ha="center", va="top", fontsize=18, fontweight="bold", color="#" + DARK)
        xs = [0.03, 0.20, 0.37, 0.54, 0.71, 0.86]
        widths = [0.12, 0.13, 0.13, 0.13, 0.12, 0.11]
        titles = ["采集", "本地门禁", "原始事件", "清洗事实", "派生投影", "产品消费"]
        subs = [
            "Git / SVN\n终端 / IDE\nAI / 浏览器",
            "授权 · DLP\n脱敏 · 项目归属\n离线队列",
            "events\n不可变 · 幂等\n可追溯",
            "clean_event_facts\n规则版本 · 合并\n隔离 · 去重",
            "Episode · 效能\n检索文档\n适配器健康",
            "活动记录\n智能查询\nLens / 导出",
        ]
        colors = [TEAL, GREEN, BLUE, AMBER, GREEN, TEAL]
        fills = [LIGHT_TEAL, LIGHT_TEAL, LIGHT_BLUE, LIGHT_AMBER, LIGHT_TEAL, LIGHT_TEAL]
        for i, x in enumerate(xs):
            box(ax, (x, 0.52), widths[i], 0.25, titles[i], subs[i], colors[i], fills[i], title_size=11, body_size=8)
            if i < len(xs) - 1:
                arrow(ax, (x + widths[i], 0.645), (xs[i + 1], 0.645))
        box(ax, (0.18, 0.13), 0.27, 0.18, "证据链与版本", "source_event_ids · rule_version\nmetric_definition_version · citations", BLUE, LIGHT_BLUE)
        box(ax, (0.57, 0.13), 0.27, 0.18, "隐私与计数边界", "AI 命令无原文 · 残片不计数\n同一操作多证据只计一次", AMBER, LIGHT_AMBER)
        arrow(ax, (0.50, 0.52), (0.32, 0.31), label="可重算")
        arrow(ax, (0.62, 0.52), (0.70, 0.31), label="强约束")

    def rag(ax):
        ax.text(0.5, 0.965, "智能查询异步处理流程", ha="center", va="top", fontsize=18, fontweight="bold", color="#" + DARK)
        box(ax, (0.04, 0.65), 0.18, 0.18, "1. 提交查询", "日期 · 设备 · 项目\n活动类型过滤", TEAL, LIGHT_TEAL)
        box(ax, (0.28, 0.65), 0.18, 0.18, "2. 问题向量", "Model Gateway\nembeddinggemma", BLUE, LIGHT_BLUE)
        box(ax, (0.52, 0.65), 0.18, 0.18, "3. 检索复核", "Qdrant top-k\nPostgreSQL 权限复核", GREEN, LIGHT_TEAL)
        box(ax, (0.76, 0.65), 0.19, 0.18, "4. 结构化生成", "qwen3:4b-instruct\n答案模式与引用约束", AMBER, LIGHT_AMBER)
        for a, b in [((0.22, 0.74), (0.28, 0.74)), ((0.46, 0.74), (0.52, 0.74)), ((0.70, 0.74), (0.76, 0.74))]:
            arrow(ax, a, b)
        box(ax, (0.10, 0.21), 0.20, 0.20, "进度状态", "queued → embedding\n→ retrieving → generating\n→ completed / failed", BLUE, LIGHT_BLUE)
        box(ax, (0.40, 0.21), 0.20, 0.20, "索引边界", "仅清洗事实\n排除残片、待确认、\nautomation 与禁用数据", TEAL, LIGHT_TEAL)
        box(ax, (0.70, 0.21), 0.20, 0.20, "输出契约", "直接答案 · 置信度\n事实/事件 ID 引用\n证据详情可打开", AMBER, LIGHT_AMBER)
        arrow(ax, (0.84, 0.65), (0.80, 0.41), label="持久化")
        arrow(ax, (0.28, 0.65), (0.20, 0.41), label="异步更新")
        ax.text(0.5, 0.08, "查询优先门让交互查询等待当前索引批次后独占模型算力，避免后台索引造成页面无响应。", ha="center", fontsize=9, color="#" + TEXT_GRAY)

    def deployment(ax):
        ax.text(0.5, 0.965, "生产服务器部署拓扑（192.168.78.138）", ha="center", va="top", fontsize=18, fontweight="bold", color="#" + DARK)
        box(ax, (0.04, 0.66), 0.22, 0.18, "外部入口 :8080", "Go Server\n/ · /admin/ · /downloads/client\n/api/v1/*", TEAL, LIGHT_TEAL)
        box(ax, (0.39, 0.66), 0.22, 0.18, "内部模型 :18081", "Model Gateway\n/internal/v1/generate\n/internal/v1/embed", BLUE, LIGHT_BLUE)
        box(ax, (0.74, 0.66), 0.22, 0.18, "内部模型 :11434", "Ollama\n生成模型 + Embedding 模型", AMBER, LIGHT_AMBER)
        box(ax, (0.11, 0.23), 0.22, 0.18, "PostgreSQL :5432", "25 GB 规划\n身份、事件、事实、任务、审计", GREEN, LIGHT_TEAL)
        box(ax, (0.39, 0.23), 0.22, 0.18, "Qdrant :6333/6334", "15 GB 规划\n仅 loopback · 768 维 Cosine", BLUE, LIGHT_BLUE)
        box(ax, (0.67, 0.23), 0.22, 0.18, "/opt/aetheris", "bin · web · config · models\nvector · backups · logs", TEAL, LIGHT_TEAL)
        arrow(ax, (0.26, 0.75), (0.39, 0.75), label="内部 Token")
        arrow(ax, (0.61, 0.75), (0.74, 0.75), label="HTTP")
        arrow(ax, (0.17, 0.66), (0.22, 0.41), label="SQL")
        arrow(ax, (0.26, 0.66), (0.50, 0.41), label="向量")
        arrow(ax, (0.61, 0.66), (0.76, 0.41), label="模型数据")
        ax.text(0.5, 0.08, "PostgreSQL、Go Server、Model Gateway、Ollama、Qdrant 均由 systemd 启用并自动随系统启动；旧 Gateway 保持禁用。", ha="center", fontsize=9, color="#" + TEXT_GRAY)

    return {
        "overall": save_diagram("01-overall-architecture.png", overall),
        "client": save_diagram("02-client-boundaries.png", client),
        "pipeline": save_diagram("03-data-pipeline.png", pipeline),
        "rag": save_diagram("04-rag-flow.png", rag),
        "deployment": save_diagram("05-deployment-topology.png", deployment),
    }


def configure_document(doc: Document) -> None:
    section0 = doc.sections[0]
    section0.top_margin = Cm(2.0)
    section0.bottom_margin = Cm(1.8)
    section0.left_margin = Cm(2.2)
    section0.right_margin = Cm(2.2)
    section0.header_distance = Cm(0.8)
    section0.footer_distance = Cm(0.8)
    section0.different_first_page_header_footer = True

    normal = doc.styles["Normal"]
    normal.font.name = "微软雅黑"
    normal._element.rPr.rFonts.set(qn("w:eastAsia"), "微软雅黑")
    normal.font.size = Pt(9.5)
    normal.font.color.rgb = RGBColor.from_string(DARK)
    normal.paragraph_format.space_after = Pt(5)
    normal.paragraph_format.line_spacing = 1.25

    title = doc.styles["Title"]
    title.font.name = "微软雅黑"
    title._element.rPr.rFonts.set(qn("w:eastAsia"), "微软雅黑")
    title.font.size = Pt(30)
    title.font.bold = True
    title.font.color.rgb = RGBColor.from_string(DARK)

    heading_sizes = {1: 18, 2: 13, 3: 10.5}
    heading_colors = {1: TEAL, 2: DARK, 3: BLUE}
    for level in (1, 2, 3):
        style = doc.styles[f"Heading {level}"]
        style.font.name = "微软雅黑"
        style._element.rPr.rFonts.set(qn("w:eastAsia"), "微软雅黑")
        style.font.size = Pt(heading_sizes[level])
        style.font.bold = True
        style.font.color.rgb = RGBColor.from_string(heading_colors[level])
        style.paragraph_format.space_before = Pt(12 if level == 1 else 8)
        style.paragraph_format.space_after = Pt(5)
        style.paragraph_format.keep_with_next = True

    for name in ("List Bullet", "List Bullet 2", "List Number"):
        style = doc.styles[name]
        style.font.name = "微软雅黑"
        style._element.rPr.rFonts.set(qn("w:eastAsia"), "微软雅黑")
        style.font.size = Pt(9.2)

    if "Caption CN" not in [s.name for s in doc.styles]:
        cap = doc.styles.add_style("Caption CN", WD_STYLE_TYPE.PARAGRAPH)
        cap.font.name = "微软雅黑"
        cap._element.rPr.rFonts.set(qn("w:eastAsia"), "微软雅黑")
        cap.font.size = Pt(8.5)
        cap.font.color.rgb = RGBColor.from_string(TEXT_GRAY)
        cap.font.italic = True
        cap.paragraph_format.alignment = WD_ALIGN_PARAGRAPH.CENTER
        cap.paragraph_format.space_after = Pt(8)

    for section_obj in doc.sections:
        header = section_obj.header
        p = header.paragraphs[0]
        p.alignment = WD_ALIGN_PARAGRAPH.RIGHT
        add_text(p, "Aetheris 产品设计与实现说明书  |  基线 0.4.16", size=8, color=TEXT_GRAY)
        footer = section_obj.footer
        add_page_number(footer.paragraphs[0])


def add_figure(doc: Document, path: Path, caption: str, width: float = 6.6) -> None:
    p = doc.add_paragraph()
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p.paragraph_format.space_after = Pt(2)
    p.add_run().add_picture(str(path), width=Inches(width))
    cp = doc.add_paragraph(style="Caption CN")
    add_text(cp, caption, color=TEXT_GRAY, size=8.5)


def add_cover(doc: Document) -> None:
    for _ in range(4):
        doc.add_paragraph()
    p = doc.add_paragraph(style="Title")
    p.alignment = WD_ALIGN_PARAGRAPH.CENTER
    add_text(p, "Aetheris", bold=True, color=DARK, size=31)
    p2 = doc.add_paragraph()
    p2.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p2.paragraph_format.space_before = Pt(8)
    add_text(p2, "产品设计与实现说明书", bold=True, color=TEAL, size=24)
    p3 = doc.add_paragraph()
    p3.alignment = WD_ALIGN_PARAGRAPH.CENTER
    p3.paragraph_format.space_before = Pt(8)
    add_text(p3, "Go Server + Windows Core + Admin Web + 本地 Lens", color=TEXT_GRAY, size=12)
    for _ in range(5):
        doc.add_paragraph()
    table = doc.add_table(rows=5, cols=2)
    table.alignment = WD_TABLE_ALIGNMENT.CENTER
    labels = [
        ("文档版本", "1.0"),
        ("产品基线", "Windows Core 0.4.16 / 清洗规则 v4 / 效能定义 v4"),
        ("适用环境", "Windows 10/11 + Linux 私有服务器"),
        ("编制日期", "2026 年 9 月 16 日"),
        ("文档语言", "中文"),
    ]
    for index, (label, value) in enumerate(labels):
        c0, c1 = table.rows[index].cells
        set_cell_shading(c0, LIGHT_TEAL)
        set_cell_shading(c1, "FAFBFB")
        set_cell_border(c0)
        set_cell_border(c1)
        p0, p1 = c0.paragraphs[0], c1.paragraphs[0]
        p0.alignment = WD_ALIGN_PARAGRAPH.CENTER
        add_text(p0, label, bold=True, color=TEAL, size=9)
        add_text(p1, value, size=9)
    doc.add_paragraph()
    note = doc.add_paragraph()
    note.alignment = WD_ALIGN_PARAGRAPH.CENTER
    add_text(note, "本文档描述当前已落地能力、兼容边界及可独立演进接口。", color=TEXT_GRAY, size=9)
    doc.add_page_break()


def build_document(diagrams: dict[str, Path]) -> Document:
    doc = Document()
    configure_document(doc)
    add_cover(doc)

    doc.add_heading("文档信息", level=1)
    paragraph(doc, "本文档面向产品、研发、测试、运维和安全评审人员，给出 Aetheris 从终端采集、服务端处理、管理分析到本地智能查询的完整产品设计与实现说明。它以当前代码库和生产部署为基线，不把尚未落地的方向描述为已交付能力。")
    add_table(doc, ["项目", "说明"], [
        ["产品目标", "把开发工作信号转换为可追溯、可纠正、隐私受控的活动事实、工作片段、个人效能指标和可引用查询答案。"],
        ["当前版本", "Windows 安装包与 Core 0.4.16；Go Server 使用 21 组增量迁移；清洗规则 v4；个人效能定义 v4。"],
        ["生产部署", "服务器 192.168.78.138，根目录 /opt/aetheris；Go Server 对外监听 8080，其余模型和向量服务仅监听 loopback。"],
        ["关键边界", "不做员工排名、不生成黑盒总分、不把活跃时段等同工时；原始事件不可变；模型不能改变基础事实和权限。"],
        ["阅读方式", "各模块章节统一描述职责、内部实现、数据、接口、主流程、失败恢复、安全边界和验收口径。"],
    ], widths=[3.2, 12.9])

    doc.add_heading("修订记录", level=2)
    add_table(doc, ["版本", "日期", "变更"], [
        ["1.0", "2026-09-16", "汇总当前全部设计、实现记录、接口契约与生产基线，形成首版完整产品说明书。"],
    ], widths=[2.0, 3.0, 11.1])

    doc.add_heading("目录", level=1)
    toc_items = [
        "1. 产品定位与设计边界", "2. 总体架构", "3. Windows Setup 安装与升级模块", "4. Windows Core 用户态采集模块",
        "5. Windows Core Service 监管模块", "6. 项目注册与项目归属模块", "7. 进程发现与授权模块", "8. 采集适配器模块",
        "9. 本地 Lens 模块", "10. Go Server 与 API 模块", "11. 身份、角色与权限模块", "12. 原始事件接收与证据存储模块",
        "13. 事件清洗与事实层模块", "14. Work Episode 工作片段模块", "15. 个人效能模块", "16. Admin Web 管理后台模块",
        "17. 智能查询与 RAG 模块", "18. Model Gateway 模块", "19. 数据存储与基础设施模块", "20. 安全、隐私与数据治理模块",
        "21. 部署、运维、备份与升级模块", "22. 可观测性、测试与验收模块", "附录 A. 主要接口", "附录 B. 核心数据表与事件类型", "附录 C. 术语与实现基线",
    ]
    for item in toc_items:
        p = doc.add_paragraph()
        p.paragraph_format.left_indent = Cm(0.4 if item[0].isdigit() else 0)
        p.paragraph_format.space_after = Pt(1.5)
        add_text(p, item, color=DARK, size=9)

    chapter(doc, 1, "产品定位与设计边界", "说明 Aetheris 要解决的问题、目标用户、能力边界和不可突破的产品约束。")
    section(doc, "1.1 产品定位")
    paragraph(doc, "Aetheris 是面向研发工作场景的隐私保护型工作信号系统。系统从用户明确授权的 Windows 终端和项目中采集结构化元数据，经本地脱敏、服务端不可变存储和可版本化清洗，形成活动记录、工作片段、个人效能指标以及带证据引用的智能查询答案。")
    paragraph(doc, "产品首先服务个人工作回顾、研发过程复盘、工具覆盖诊断和受控知识检索；管理员获得的是可解释事实及数据质量，而不是对员工进行自动排名或绩效评判的工具。")
    section(doc, "1.2 核心角色")
    add_table(doc, ["角色", "主要诉求", "可见范围"], [
        ["终端用户", "安装、授权项目和进程、查看本机状态、纠正或删除本机证据。", "当前 Windows 用户与当前设备。"],
        ["租户管理员", "管理设备、主体、工作角色、策略、项目、采集覆盖和数据质量。", "所属租户内授权范围。"],
        ["分析使用者", "查看活动、工作片段、个人效能和智能查询结果。", "由权限和项目范围共同限定。"],
        ["运维人员", "部署、备份、升级、监控磁盘与系统服务。", "基础设施与运行状态；日志不含业务正文。"],
        ["模型服务", "对最小必要投影执行向量化或文本生成。", "无数据库直连、无设备凭据、无原始未授权数据。"],
    ], widths=[2.7, 7.5, 5.8])
    section(doc, "1.3 产品原则")
    for item in [
        "先授权后采集：未知进程、未授权项目和非白名单浏览器页面不能形成可上传内容。",
        "原始证据不可变：纠正使用 supersession，删除使用 tombstone，派生数据可以重算但不能回写原始事实。",
        "确定性优先：项目归属、命令分类、指标计算和权限判断优先采用可复现规则；模型只做受约束的增强。",
        "边界清晰：Setup、Core、Service、Lens、Go Server、Admin Web 和 Model Gateway 均有独立构建或启动入口。",
        "按最小必要原则处理内容：绝对路径、凭据、截图、完整 AI 命令和源代码正文均受到严格限制。",
        "可解释而非评分：指标必须带口径、覆盖度和证据，不提供员工排名、黑盒总分或奖惩判断。",
    ]:
        add_bullet(doc, item)
    add_callout(doc, "已确认的产品边界", "“活跃时段”仅表示包含有效活动事实的五分钟窗口；没有动作的五分钟窗口不计入。它不等于工时、在线时长或加班时长。", "warning")

    chapter(doc, 2, "总体架构", "给出四大独立组件及数据处理模块之间的关系、部署边界和演进方式。")
    add_status_strip(doc, "跨组件合同与运行拓扑", "Windows + Linux 私有部署", "生产运行", "AetherisEvent / OpenAPI / 内部模型 API")
    add_figure(doc, diagrams["overall"], "图 2-1  Aetheris 总体运行架构")
    section(doc, "2.1 四个独立交付组件")
    add_table(doc, ["组件", "职责", "独立入口", "耦合控制"], [
        ["Go Server", "认证、设备 API、Admin API、事件接收、PostgreSQL 持久化、后台派生任务和静态文件托管。", "aetheris-server / aetheris-migrate / aetheris-admin", "不导入前端和模型网关实现；只通过文件目录与内部 HTTP 对接。"],
        ["Windows Core", "本地授权、采集、脱敏、队列、心跳、上传、托盘与本地控制。", "AetherisCore.exe", "不直连数据库；只调用版本化设备 API。"],
        ["Admin Web", "登录后管理、分析和智能查询。", "npm run dev / npm run build", "不直接访问 PostgreSQL；不承担最终授权判断。"],
        ["Local Lens", "本机状态、项目、进程授权、适配器健康和个人证据。", "随 Core 在 loopback 暴露", "只能访问当前用户/设备；不成为远程管理入口。"],
    ], widths=[2.3, 5.2, 3.5, 5.1])
    section(doc, "2.2 领域模块")
    paragraph(doc, "在四个交付组件内部，Aetheris 继续沿用 Core、Lens、Forge、Nexus、Pulse 的领域划分：Core 负责采集，Lens 负责可见和修正，Forge 负责清洗与工作片段，Nexus 负责检索与模型编排，Pulse 负责可解释分析。这些是职责边界，不要求首版拆成独立网络微服务。")
    section(doc, "2.3 数据主链路")
    add_figure(doc, diagrams["pipeline"], "图 2-2  从本地信号到产品消费的数据链路")
    paragraph(doc, "终端侧只有通过项目授权、进程授权、DLP 和脱敏的事件才能进入本地 SQLite 队列；Go Server 接收后先保存不可变原始事件，再由独立 Worker 生成清洗事实、项目归属、工作片段、个人效能日聚合和检索文档。任何派生失败都不能阻塞 heartbeat 和原始事件入库。")
    section(doc, "2.4 兼容与演进")
    for item in [
        "旧版 /v1/events 和旧 AI 交互 API 只做兼容，不作为新页面的主数据源。",
        "对象存储和向量索引通过接口接入；PostgreSQL 保持事务和权限事实源。",
        "Dify 是可选模型编排器，不拥有 Aetheris 数据、索引、引用或权限。",
        "规则升级通过版本指针和重算切换，不通过修改历史事件完成。",
    ]:
        add_bullet(doc, item)

    return doc


def add_remaining_chapters(doc: Document, diagrams: dict[str, Path]) -> None:
    chapter(doc, 3, "Windows Setup 安装与升级模块", "描述独立安装程序如何完成目录选择、项目发现、设备注册、凭据保护、服务安装、验收和回滚。")
    add_status_strip(doc, "终端交付与首次配置", "原生 NSIS + Provisioning DLL", "0.4.16", "Bootstrap / Heartbeat / SCM API")
    section(doc, "3.1 模块职责与交付物")
    paragraph(doc, "Windows Setup 是唯一面向用户的安装入口，独立于 Core。安装包为单个 NSIS EXE，内嵌用户态 AetherisCore.exe、原生 AetherisCoreService.exe、Provisioning 插件和可选 VS Code VSIX。用户可以自行选择 Core 安装目录；只有 LocalSystem 服务二进制固定放置在受保护的 Program Files 目录。")
    add_table(doc, ["交付物", "安装位置", "用途"], [
        ["AetherisCore.exe", "用户选择的安装目录", "托盘、采集、本地 Lens、队列和设备通信。"],
        ["AetherisCoreService.exe", "%ProgramFiles%\\Aetheris\\Service", "Windows 启动与会话监管，不处理采集内容。"],
        ["AetherisProvisioning.dll", "安装 staging，完成后清理", "原生 HTTP、DPAPI、文件、项目扫描和 SCM 操作。"],
        ["Aetheris VS Code 扩展", "VS Code 用户扩展目录", "可选采集真实文件行为元数据。"],
        ["Uninstall.exe", "用户选择的安装目录", "撤销服务与设备凭据，并按用户选择保留或清理数据。"],
    ], widths=[3.4, 5.0, 7.6])
    section(doc, "3.2 安装向导")
    for item in [
        "选择安装目录：允许用户使用非默认目录，不把 Core 强制安装到 LOCALAPPDATA。",
        "选择项目扫描根：在后台递归查找 Git 或 SVN 标记，界面显示进度，不阻塞 UI 线程。",
        "确认项目：扫描结果按路径去重并默认全选，用户可以保留多个项目；扫描跳过依赖、构建、缓存和 VCS 内部目录。",
        "确认用户标识：默认 DOMAIN\\USERNAME；一个 Windows 终端绑定一个主体，一个主体可拥有多台终端。",
        "输入 enrollment code：该码由服务端管理员提供，是一次性注册密钥，不是设备 Token。",
        "可选安装 VS Code 扩展：默认选中但允许取消，选择结果进入组件状态。",
        "执行安装与验收：只有 bootstrap、DPAPI 写入、heartbeat、Windows 服务 Running、Core ready 和设备可见全部通过才显示成功。",
    ]:
        add_number(doc, item)
    section(doc, "3.3 项目扫描实现")
    paragraph(doc, "Provisioning 采用后台工作线程扫描目录，NSIS 页面定时读取扫描状态并更新进度条。扫描识别目录形式或文件形式的 .git，以及目录形式的 .svn；最大深度默认为 8，结果上限为 100，并避开 junction/reparse point，防止循环和长时间卡死。")
    add_code(doc, r'''
scan_root: E:\code
markers: .git (directory or file), .svn (directory)
skip: .git, .svn, node_modules, .venv, dist, build, bin, obj, .cache
deduplicate: case-insensitive normalized Windows path
result: [{path, vcs, selected}]
''')
    section(doc, "3.4 Enrollment 与凭据")
    paragraph(doc, "Setup 调用 POST /api/v1/device/bootstrap 交换一次性的 enrollment code，服务端签发 device token 后只保存其哈希。终端立即使用 Windows DPAPI CurrentUser 加密 token，写入 ACL 受限的 device.credential。Setup 随即调用 heartbeat；响应中的 device_id、subject_id 必须与本地配置一致。")
    add_callout(doc, "成功口径", "“文件复制完成”不是安装成功。安装成功以 heartbeat 成功、服务端设备列表可见、服务 Running 且 Core 发出 ready 为准。", "warning")
    section(doc, "3.5 升级、身份复用与回滚")
    paragraph(doc, "升级优先验证现有配置和 DPAPI 凭据；凭据仍可 heartbeat 时复用原设备身份，不重复显示 enrollment。升级前由服务 IPC 请求 Core prepare_update，并保存同卷 previous 副本。新服务或 Core 未在时限内 ready 时恢复旧二进制和 HKLM 配置，保留 config、credential、SQLite 和项目授权。")
    section(doc, "3.6 异常处理")
    add_table(doc, ["异常", "用户可见行为", "系统处理"], [
        ["Enrollment 401/403", "明确提示注册码无效", "不创建成功状态；保留输入页，可重新提交。"],
        ["网络/服务端不可达", "显示阶段和可重试错误", "不删除已复制文件和旧配置；新装不宣告成功。"],
        ["项目扫描过慢", "持续显示进度与当前数量", "后台线程继续，UI 保持响应；可返回上一步。"],
        ["服务创建失败", "显示稳定错误码", "清理本次创建的服务和 staging，不递归删除未知文件。"],
        ["升级验收失败", "显示已回滚", "恢复 previous、重启旧服务并保留用户数据。"],
    ], widths=[3.0, 5.1, 7.9])
    section(doc, "3.7 验收标准")
    for item in [
        "在用户选择目录完成安装后，即使删除下载目录，Core 仍可启动。",
        "安装包、Core、服务和上报版本一致；当前基线为 0.4.16。",
        "错误 enrollment code、heartbeat 失败或设备不可见时不能显示安装成功。",
        "安装结束后安装器进程退出，仅保留服务、Core 和用户已打开的 VS Code 等正常进程。",
        "安装和升级流程不通过 cmd.exe、PowerShell、sc.exe 或 schtasks.exe 完成系统操作。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 4, "Windows Core 用户态采集模块", "描述 Core 的本地运行时、调度、离线队列、状态机、策略同步和服务端通信。")
    add_status_strip(doc, "终端采集与隐私执行", "Python 打包为 windowed one-file EXE", "0.4.16", "Device API / SQLite / DPAPI / Local Lens")
    add_figure(doc, diagrams["client"], "图 4-1  Windows 客户端组件与权限边界")
    section(doc, "4.1 启动与生命周期")
    paragraph(doc, "Core 运行在当前交互用户身份下，由 Windows 服务在活动控制台会话启动。启动时获取单实例互斥量，加载配置、DPAPI 凭据、项目注册表、进程授权和本地队列；随后启动托盘、本地 Lens、heartbeat、策略同步和适配器调度。第二实例只打开现有状态页并退出。")
    section(doc, "4.2 运行状态模型")
    add_table(doc, ["状态域", "关键字段", "规则"], [
        ["registration", "state、last_heartbeat_at、last_error", "注册错误不能被采集成功清除；401/403 标记 credential_invalid。"],
        ["capture", "state、project_count、last_capture_at、last_error", "单个适配器失败不停止其他适配器。"],
        ["queue", "queued、retrying、rejected", "网络故障保留事件；字段错误进入拒绝状态，不无限重试。"],
        ["browser_policy", "enabled、revision、domain_count、last_error", "同步失败继续使用最后有效策略，不记录域名明细。"],
        ["server_sync", "server_event_count、data_generation", "检测服务端代际下降后重置本地 AI 历史 checkpoint 并幂等重放。"],
    ], widths=[2.6, 6.4, 7.0])
    section(doc, "4.3 Heartbeat 与重试")
    paragraph(doc, "Core 启动后立即 heartbeat，成功后默认每 60 秒执行一次。网络错误、超时和 5xx 使用从 5 秒到 5 分钟的指数退避；401/403 保留 credential_invalid 并每 5 分钟重试；参数类 4xx 标记配置错误并停止高频重试。Heartbeat 与 ingest 是独立状态链路。")
    section(doc, "4.4 本地队列")
    paragraph(doc, "已脱敏事件先写入 SQLite，再批量上传。队列使用 claim/ack 语义保证崩溃恢复和幂等；本地总空间默认上限 512 MB，近期缓存保留 7 天，未确认事件优先保留。接近配额时暂停新采集并产生本地诊断，不静默丢弃。")
    section(doc, "4.5 调度与适配器隔离")
    paragraph(doc, "适配器按独立健康状态运行。浏览器采集被提前到采集周期前部，避免 Git 或 AI 历史扫描拖延前台窗口采样；AI 历史采用实时通道和公平回填通道，避免少数活跃大文件长期占满额度。适配器输出统一交给项目归属、DLP、脱敏、事件封装和队列。")
    add_callout(doc, "0.4.16 公平回填", "Codex 会话扫描保留实时额度给最近会话，同时用持久 backfill_cursor 轮转全部文件；每个大文件单轮有上限，Core 重启后从游标继续。这样 safe、Desktop-Asist 等不活跃项目不会因活跃会话占满 500 条额度而长期缺失。", "success")
    section(doc, "4.6 托盘与用户控制")
    for item in [
        "托盘显示注册、采集和队列摘要，并提供打开状态页、立即 heartbeat、立即采集和退出。",
        "首次成功启动显示 Windows 通知并自动打开一次本地 Lens。",
        "用户主动退出通过服务 IPC 发送 authenticated normal_exit；服务在当前登录会话内不自动拉起。",
        "用户从开始菜单重新启动时发送 resume，清除本次会话抑制并重新接受服务监管。",
    ]:
        add_bullet(doc, item)
    section(doc, "4.7 安全实现")
    paragraph(doc, "Core 不上传未授权路径；绝对路径只用于本机项目匹配。设备 token、控制 token、项目身份密钥和命令指纹密钥均通过 DPAPI CurrentUser 保护。日志只记录稳定错误码、事件 ID 和计数，不记录事件 payload、命令正文、OCR 文本、URL、token 或项目绝对路径。")

    chapter(doc, 5, "Windows Core Service 监管模块", "说明原生 Windows 服务如何随系统启动、进入正确用户会话、监管 Core，并保持严格的 LocalSystem 权限边界。")
    add_status_strip(doc, "启动与进程监管", "原生 C++ Windows Service", "Automatic Delayed Start", "SCM / WTS / 受保护命名管道")
    section(doc, "5.1 设计动机")
    paragraph(doc, "目标终端策略会阻止 Task Scheduler 注册，因此当前版本采用原生 Windows 服务。服务只负责当前活动控制台用户的 Core 生命周期，避免把截图、DPAPI、窗口枚举和采集逻辑放入 LocalSystem。")
    section(doc, "5.2 会话启动")
    paragraph(doc, "服务启动后读取受保护 HKLM 配置，识别活动控制台 SessionId，校验安装用户 SID，通过 WTSQueryUserToken、DuplicateTokenEx、CreateEnvironmentBlock 和 CreateProcessAsUserW 在 winsta0\\default 启动 Core。Core 使用用户的非提升令牌，从而获得正确的 DPAPI 和交互桌面。")
    section(doc, "5.3 服务 IPC")
    add_table(doc, ["消息", "方向", "用途"], [
        ["ready", "Core → Service", "配置、凭据和本地状态已经完成初始化。"],
        ["heartbeat", "Core → Service", "仅证明进程响应，不携带采集统计和正文。"],
        ["normal_exit", "Core → Service", "与 10 秒内退出码 0 共同构成用户主动退出。"],
        ["resume", "Core → Service", "手工启动时清除当前会话抑制。"],
        ["prepare_update / update_ready", "Setup ↔ Core", "升级前安全停止并刷新状态。"],
        ["session_stop", "Service → Core", "活动控制台用户切换或注销时停止旧会话实例。"],
    ], widths=[4.3, 3.1, 8.6])
    paragraph(doc, "命名管道为 \\.\\pipe\\Aetheris.Core.Service.v1，帧上限 16 KiB。连接必须同时通过 PID、进程令牌 SID 和 SessionId 校验；协议不接受任意命令、路径、环境变量或 shell 参数。")
    section(doc, "5.4 恢复与抑制")
    add_table(doc, ["场景", "行为"], [
        ["Core 异常退出少于 5 次/10 分钟", "60 秒后恢复。"],
        ["达到 5 次/10 分钟", "进入 15 分钟 crash_loop_backoff。"],
        ["连续健康 30 分钟", "清空失败计数。"],
        ["用户主动退出", "当前登录会话标记 user_suppressed；服务重启仍遵守。"],
        ["用户注销或 Windows 重启", "清除退出抑制，新会话重新自动启动。"],
        ["服务自身异常退出", "SCM 在 60 秒后恢复服务，24 小时后重置失败计数。"],
    ], widths=[6.0, 10.0])
    section(doc, "5.5 安全边界与验收")
    for item in [
        "服务不加载 Python、不访问网络、不打开 SQLite、credential、项目文件、截图或 OCR 文本。",
        "服务二进制和 HKLM 配置只允许 SYSTEM 和 Administrators 修改；用户可修改 Core 不会获得 LocalSystem 权限。",
        "强制结束 Core 后应在恢复窗口出现新 PID；托盘主动退出后至少 120 秒不得自动重启。",
        "系统重启后服务为 Running，Core 位于当前用户会话，托盘和 Lens 可访问。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 6, "项目注册与项目归属模块", "描述本地项目发现、跨设备逻辑项目身份、AI 工具兜底项目、历史归属回填和项目展示规则。")
    add_status_strip(doc, "项目身份与归属", "Core + Go Server + PostgreSQL", "已落地", "Device Projects API / Project Admin API")
    section(doc, "6.1 两级项目模型")
    paragraph(doc, "本地项目位置与逻辑项目分离。项目位置表示某台设备上的授权工作目录；逻辑项目表示跨设备、跨克隆目录的稳定项目。相同 Git 远程仓库通过租户级有键指纹归并，目录名称相同但远程不同的项目不会自动合并。")
    add_table(doc, ["对象", "核心标识", "保存内容", "隐私要求"], [
        ["本地项目位置", "local_project_id / root_fingerprint", "设备、VCS、末级名称、工作树关系、授权状态。", "绝对路径只在设备本地。"],
        ["逻辑项目", "logical_project_id", "安全显示名、归并状态、冲突状态。", "不包含本机盘符和用户目录。"],
        ["项目归属", "event_id + rule_version", "逻辑项目、位置、方法、置信度和安全证据。", "不复制路径或远程 URL 原文。"],
        ["工具兜底项目", "project-tool-<tool>", "Codex、Claude Code、Cursor、GitHub Copilot 等逻辑标签。", "cwd 无法授权映射时不上传 cwd。"],
    ], widths=[3.0, 3.5, 6.0, 3.6])
    section(doc, "6.2 本地项目身份")
    paragraph(doc, "Core 对 Git HTTPS、SSH 和 SCP 风格 remote 做规范化，再用租户用途独立密钥生成远程指纹；设备根目录另用设备密钥生成位置指纹。项目身份密钥由注册设备通过专用 API 获取并用 DPAPI 缓存，网络失败时可使用仍有效的缓存。")
    section(doc, "6.3 AI 项目归属")
    for item in [
        "AI 会话 cwd 位于已授权 Git/SVN 根目录内时，使用该项目稳定 ID和仓库末级显示名。",
        "cwd 缺失、位于临时目录或授权范围外时，归入工具逻辑项目，不丢弃结构化 AI 数据。",
        "fallback payload 只包含 project_label 和 project_attribution，不上传完整 cwd。",
        "历史修复使用 supersedes_event_id 关联旧归因证据，清洗层采用当前证据。",
    ]:
        add_bullet(doc, item)
    section(doc, "6.4 历史归属规则")
    add_table(doc, ["优先级", "方法", "自动处理条件"], [
        ["1", "exact_binding", "事件本地项目 ID与设备项目位置精确绑定。"],
        ["2", "remote_fingerprint / safe_label", "安全远程指纹或已确认项目标签唯一匹配。"],
        ["3", "authorized_root", "历史安全投影能确定授权根。"],
        ["4", "session_correlation", "同一 AI session 内存在唯一高置信度项目证据。"],
        ["5", "time correlation", "同设备、项目位置和限定时间窗口候选唯一。"],
        ["6", "tool fallback / unresolved", "无法可靠归属时保留兜底标签并进入复核，不强制映射。"],
    ], widths=[2.0, 5.0, 9.0])
    section(doc, "6.5 回填、启用与回滚")
    paragraph(doc, "项目回填支持 dry-run、apply、resume 和 activate。计算结果按 rule_version 并存，只有完成的 apply 任务才能激活。启用新版本后依次重算清洗事实、工作片段、检索文档、向量和效能；回滚只切换当前版本指针，不改写 events。")
    section(doc, "6.6 展示与验收")
    paragraph(doc, "正常界面只显示 logical_projects.display_name，不显示裸 project-* 或本机完整路径。未归属数据显示“未归属 · Codex”等安全中文名称。验收必须覆盖同仓库多克隆归并、同名不同仓库不归并、工作树归属、工具兜底和历史回填幂等。")

    chapter(doc, 7, "进程发现与授权模块", "说明进程身份、一次提醒、授权状态、项目范围和本地隐私边界。")
    add_status_strip(doc, "本地进程治理", "Windows Core + Local Lens + SQLite", "0.4.9 起使用名称+路径", "Local Process Consent API")
    section(doc, "7.1 进程身份")
    paragraph(doc, "本期严格使用“规范化进程名称 + 规范化可执行路径”识别同一进程。Windows 路径统一分隔符、折叠 . 和 .. 并忽略大小写。发布者、签名状态和文件哈希仍作为安全元数据更新，但不再参与主键，避免程序升级后出现大量同名重复授权项。")
    section(doc, "7.2 授权状态")
    add_table(doc, ["状态", "含义", "是否采集"], [
        ["pending", "新发现、等待用户决定。", "否"],
        ["allow_global", "对所有已授权项目视为工作相关。", "满足项目和适配器规则时是"],
        ["allow_project", "只对指定逻辑项目允许。", "仅指定项目"],
        ["deny", "当前不监控，保留在已处理列表。", "否"],
        ["always_ignore", "持续忽略，直到用户重置。", "否"],
        ["default_excluded", "系统、安全、即时通信、打印或 Aetheris 自身默认排除。", "否"],
    ], widths=[3.1, 8.1, 4.8])
    section(doc, "7.3 用户流程")
    for item in [
        "原生发现器观察活动用户会话中的进程身份。",
        "先应用默认排除，再查询本地 SQLite 中已有决定。",
        "新 pending 身份只产生一次托盘通知，并持续保留在 Lens 待处理列表。",
        "用户可单条或批量选择工作相关、仅当前项目、不监控、永久忽略或重置。",
        "只有允许决定和有效项目上下文同时存在，内容采集才可继续。",
    ]:
        add_number(doc, item)
    section(doc, "7.4 项目关联")
    paragraph(doc, "允许的进程按“显式项目范围授权 → 适配器工作区/解决方案 → 父进程项目上下文 → 唯一活动项目 → 待归类复核”解析项目。存在歧义时阻止内容采集，但不把它误报为安装失败。")
    section(doc, "7.5 本地数据边界")
    paragraph(doc, "授权决定仅保存在当前用户本地。服务端可以接收待确认、允许和忽略的汇总数量，但不能接收用户拒绝的个人应用清单。Admin Web 不提供远程审批本地进程的能力。")

    chapter(doc, 8, "采集适配器模块", "逐类说明终端、版本控制、IDE、AI、浏览器和 OCR 保底适配器的输入、输出与隐私边界。")
    add_status_strip(doc, "多源研发活动采集", "Core adapters + VS Code extension", "逐适配器独立健康", "AetherisEvent / AdapterHealthSnapshot")
    section(doc, "8.1 统一适配器合同")
    paragraph(doc, "每个适配器必须具备格式检测、只读访问、持久 checkpoint、有界解析、落盘前脱敏、安全健康快照和真实格式测试。适配器故障只更新自身状态，不阻塞其他采集器、heartbeat 或队列上传。")
    add_table(doc, ["适配器", "主要来源", "事件/事实", "不采集内容"], [
        ["Git", "git CLI 只读命令", "commit 元数据、diff 数量、版本控制活动", "patch body、仓库凭据、原始远程 URL"],
        ["SVN", "svn info/status --xml", "working-copy revision、状态数量", "密码、文件正文、repository URL 原文"],
        ["PowerShell/Terminal", "PSReadLine 历史", "terminal.command、逻辑命令和残片证据", "终端输出、环境变量、未脱敏凭据"],
        ["Codex/Claude Code", "本地结构化 JSONL", "ai.message、ai.tool_call、session 元数据", "AI 命令原文、动态 arguments、未授权 cwd"],
        ["Cursor/Copilot", "受支持的本地结构化存储", "AI 会话与安全消息投影", "私有数据库无关字段、凭据"],
        ["VS Code 窗口", "原生顶层窗口枚举", "ide.activity 低粒度工作区/活动文件变化", "源代码正文、后台文档内容"],
        ["VS Code 扩展", "VS Code 官方 API", "file opened/edited/saved/closed、workspace、extension change", "输入字符、代码正文、完整 diff、剪贴板"],
        ["Visual Studio", "解决方案、窗口与可用事件源", "visualstudio.activity、构建/调试/测试元数据", "源文件正文和调试敏感内容"],
        ["Browser", "Chrome/Edge 前台窗口、UIA、内存截图和 OCR", "browser.page_view", "历史记录、Cookie、表单值、非白名单页面"],
        ["Application OCR", "已授权前台进程的受控截图", "application.activity 低置信度事实", "截图文件、高风险 OCR 原文、密码窗口"],
    ], widths=[2.4, 3.4, 5.0, 5.2], font_size=7.7)
    section(doc, "8.2 PowerShell 逻辑命令")
    paragraph(doc, "终端适配器先按奇数尾随反引号拼接显式续行，再用 PowerShell AST 完整性校验。以参数开头且无法独立执行的记录，会在同设备、同项目、相邻顺序和时间窗口内尝试与主命令合并；倒序样本可重排后校验。无法可靠合并的片段保留为原始证据并排除效能。")
    section(doc, "8.3 AI 会话与工具调用")
    paragraph(doc, "只解析官方本地历史中的结构化消息和工具调用，不从自然语言回复猜测命令。AI shell 工具只生成 command_type、安全摘要和设备内 HMAC；无法静态确定的自动化调用进入待确认。Codex 历史采用实时 + 公平回填双通道，保证多项目会话逐步全覆盖。")
    section(doc, "8.4 VS Code 行为")
    paragraph(doc, "真实文件编辑行为由独立 VS Code 扩展通过当前用户 ACL 的命名管道发送给 Core；扩展没有设备 Token，不直接访问服务端。编辑事件只统计变更次数和新增/删除字符数量，contentChanges.text 只在回调内读取长度。Core 完成项目授权、相对路径化和绝对路径删除后才入队。")
    section(doc, "8.5 浏览器活动")
    paragraph(doc, "浏览器采集默认关闭。管理员启用并配置规范域名白名单后，Core 才处理 Chrome/Edge 前台窗口。UIA 读取地址栏并验证域名，截图只存在内存，用于有限滚动和 OCR；脱敏/DLP 完成后立即释放。非白名单、后台浏览器、ChatGPT 桌面应用或 Explorer 等前台进程不会生成浏览器页面事件。")
    section(doc, "8.6 通用应用 OCR 保底")
    paragraph(doc, "对已经本地授权、位于前台且 60 秒没有专属适配器事件的工作进程，可以触发低频 OCR 保底。密码控件、浏览器、安全工具、即时通信和隐私门禁命中的窗口永久排除；相同内容五分钟内只上传一次。该事实置信度为 low，并在同一时段已有专属事实时不重复增加效能活跃窗口。")
    section(doc, "8.7 适配器健康")
    add_table(doc, ["状态", "说明"], [
        ["active", "近期成功产生有效事件。"], ["idle", "数据源正常但当前没有变化。"],
        ["disabled", "用户取消、停用或策略关闭。"], ["source_missing", "所需应用或数据源不存在。"],
        ["permission_denied", "只读访问被操作系统拒绝。"], ["format_changed", "检测到未知数据格式，停止猜测解析。"],
        ["dependency_missing", "Git/SVN/OCR 等外部依赖缺失。"], ["error", "协议、解析、队列或运行错误，附稳定错误码。"],
    ], widths=[4.0, 12.0])

    chapter(doc, 9, "本地 Lens 模块", "描述只在本机回环地址开放的状态、授权、项目和证据管理体验。")
    add_status_strip(doc, "本机可见、纠正与控制", "Core 内嵌 HTTP + 静态页面", "优先 127.0.0.1:15473", "Local API / Control Credential / CSRF")
    section(doc, "9.1 定位与访问边界")
    paragraph(doc, "Local Lens 是终端用户的第一方控制面，随 Core 启动，优先绑定 127.0.0.1:15473；端口冲突时选择动态端口并写入状态文件。它拒绝非 loopback Host，不暴露到服务端，也不能访问其他用户或设备数据。")
    section(doc, "9.2 页面结构")
    add_table(doc, ["页面/区域", "内容", "可执行操作"], [
        ["总览", "服务、Core、注册、采集、队列和最近错误。", "立即 heartbeat、立即采集、启动或退出 Core。"],
        ["项目", "授权项目、VCS、逻辑项目、状态和 revision。", "添加、暂停、恢复、移除、解决冲突。"],
        ["进程授权", "状态统计、筛选、搜索、名称、路径、发布者、决定时间。", "单条/批量允许、项目允许、拒绝、永久忽略、重置。"],
        ["采集健康", "每个适配器状态、阶段、计数、最近事件和错误码。", "针对性恢复或重新检查。"],
        ["VS Code 采集", "扩展安装状态、版本、协议和最近事件。", "安装、启用、停用、卸载、清除缓存。"],
        ["个人工作片段", "当前设备的目标、动作、验证、结果和证据。", "查看证据、纠正或发起替代。"],
    ], widths=[3.0, 7.2, 5.8])
    section(doc, "9.3 本地控制安全")
    paragraph(doc, "读操作受 loopback 和本地会话限制；所有变更操作要求独立于设备 token 的 local control credential 和 CSRF 校验。页面不把 token 放入 URL 或日志，服务端 Admin 会话也不能用于调用 Local Lens。")
    section(doc, "9.4 进程授权工作台")
    paragraph(doc, "进程列表按名称+路径去重，提供待确认/已处理状态筛选以及名称、路径、发布者搜索。批量操作逐条提交，部分失败时保留成功项并单独显示失败项，刷新后以本地数据库最终状态为准。")
    section(doc, "9.5 错误与恢复")
    for item in [
        "固定端口不可用时使用动态端口，托盘始终打开状态文件中记录的真实 URL。",
        "页面脚本错误必须被前端回归和真实浏览器点击验证覆盖，不能仅检查 HTTP 200。",
        "Core 停止时页面不可用属于预期；Windows 服务状态仍可从服务日志和 SCM 检查。",
        "项目操作采用 revision 防止并发覆盖；冲突返回当前版本供页面刷新。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 10, "Go Server 与 API 模块", "说明 Go Server 的进程入口、包边界、路由、后台 Worker、静态文件托管和失败隔离。")
    add_status_strip(doc, "服务端业务与 API", "Go + PostgreSQL", "生产 :8080", "/api/v1/* /admin/* /downloads/client")
    section(doc, "10.1 独立入口")
    add_table(doc, ["入口", "用途"], [
        ["aetheris-server", "启动 HTTP、数据库连接池、后台 Worker、静态 Admin Web 和客户端下载。"],
        ["aetheris-migrate", "按版本执行 PostgreSQL 增量 migration。"],
        ["aetheris-admin", "管理员密码重置、清洗历史回填等受控运维操作。"],
    ], widths=[4.1, 11.9])
    section(doc, "10.2 内部分层")
    paragraph(doc, "HTTP handler 只负责协议解析、参数校验、认证和序列化；authorization 负责统一 permission 和 scope 判断；领域 service 执行业务规则；repository 负责带 tenant scope 的 SQL；worker 处理清洗、项目归属、工作片段、效能、索引和查询任务。领域包不依赖 React 或具体模型 provider。")
    add_table(doc, ["包域", "职责"], [
        ["auth / authorization", "管理员会话、密码、权限和租户/项目范围。"],
        ["devices / projects / browserpolicy / applicationpolicy", "设备注册、项目同步和终端策略。"],
        ["events / activities / cleaning", "原始事件校验、统一活动查询和事实清洗。"],
        ["projectattribution / episodes", "历史归属、版本启用和工作片段。"],
        ["effectiveness", "日聚合、趋势、覆盖度和模型总结投影。"],
        ["retrieval", "检索文档、Qdrant、异步查询、结构化答案和引用。"],
        ["adapterhealth / health / adminops", "采集覆盖、运行健康和受控运维。"],
    ], widths=[6.1, 9.9])
    section(doc, "10.3 路由与静态托管")
    paragraph(doc, "根路径直接进入登录页；登录后 Admin Web 显示主页面，右上角提供 Windows 客户端下载。Go Server 只提供 API 和静态文件托管，不使用 Go 模板渲染。/downloads/client 只返回配置指定的单个已验证安装包，不开放目录遍历或任意文件读取。")
    section(doc, "10.4 后台 Worker")
    add_table(doc, ["Worker", "触发", "失败影响"], [
        ["Cleaning", "原始事件增量或人工范围重算", "不影响 ingest；任务记录错误并可重跑。"],
        ["Project Attribution", "项目注册变化或 backfill job", "继续使用上一启用版本。"],
        ["Episode", "新事实或归属切换", "保留上一有效 revision。"],
        ["Effectiveness", "每 5 分钟重算今天/昨天或人工任务", "页面显示旧聚合和任务状态。"],
        ["Retrieval Indexer", "当前规则版本事实增量", "文档标记 failed/pending，不阻塞活动查询。"],
        ["RAG Query", "用户提交异步查询", "状态 failed，保留错误码和已授权过滤条件。"],
    ], widths=[4.0, 6.0, 6.0])
    section(doc, "10.5 API 共同约束")
    for item in [
        "所有设备身份从 token 反查，客户端传入的 tenant/subject/device 不作为权威。",
        "所有 Admin 查询都必须有 tenant scope，项目范围在 repository 和 PostgreSQL RLS 两层约束。",
        "JSON 请求有大小上限，时间范围和分页参数有上限，避免全表扫描和内存加载。",
        "写操作进入 audit_logs，日志不记录 payload、token、原始 prompt 或模型 response。",
        "请求失败返回稳定错误码与 request ID；内部依赖错误不把凭据和实现堆栈返回浏览器。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 11, "身份、角色与权限模块", "区分管理员访问权限与被采集用户工作角色，说明主体、终端、会话和权限判定。")
    add_status_strip(doc, "身份与访问控制", "Go Server + PostgreSQL RLS", "已落地", "Session Cookie / Device Token / Permission")
    section(doc, "11.1 身份关系")
    paragraph(doc, "一个 Windows 终端只对应一个采集主体，一个主体可以拥有多台终端。Setup 依据规范化用户标识派生稳定 subject_id，并结合机器标识生成 device_id。服务端 bootstrap 创建或绑定主体与设备；已存在主体不会因另一台设备注册而静默改名。")
    add_table(doc, ["实体", "含义", "基数"], [
        ["tenant", "组织和数据隔离边界。", "拥有多个用户、主体、设备和项目。"],
        ["user", "登录 Admin Web 的账户。", "通过 membership 获得访问角色。"],
        ["subject", "被采集的实际终端用户。", "一个主体可绑定多台设备。"],
        ["device", "一台 Windows 终端。", "第一版只能绑定一个主体。"],
        ["work_role", "研发、测试、产品等业务工作身份。", "可分配到主体或项目范围。"],
        ["access_role", "platform_admin、tenant_admin 等系统访问角色。", "决定管理端权限，不代表工作身份。"],
    ], widths=[3.2, 6.2, 6.6])
    section(doc, "11.2 两类角色严格分离")
    add_callout(doc, "关键概念", "“研发、测试、产品”是终端采集用户的工作角色，用于数据归类；“tenant_admin、member、device_ingest”是系统访问角色，用于授权。两者不能混用。", "warning")
    paragraph(doc, "工作角色生效优先级为项目覆盖、主体分配、租户默认值。事件入库时固化 role_id、code、version 和 source 快照，因此后续把主体改为研发不会悄悄改写历史证据；需要历史口径调整时通过受审计的派生重算完成。")
    section(doc, "11.3 访问角色与权限")
    add_table(doc, ["访问角色", "典型权限"], [
        ["platform_admin", "跨租户平台管理，仅限受控场景。"],
        ["tenant_admin", "租户设备、主体、角色、策略、项目、活动、效能和运维任务。"],
        ["analyst / reviewer", "按授权项目或范围读取事实、证据和审计；默认不读取完整个人效能。"],
        ["member", "通过 user_subject_links 显式绑定后读取本人数据。"],
        ["device_ingest", "仅 bootstrap 后的 heartbeat、项目注册、健康和事件上传；不能读取管理数据。"],
    ], widths=[4.2, 11.8])
    section(doc, "11.4 会话与设备凭据")
    paragraph(doc, "管理员登录使用短期会话与刷新机制，浏览器只保存 HttpOnly、SameSite cookie；所有 cookie 认证的写操作要求 CSRF。设备 token 只在 bootstrap 响应一次，服务端保存哈希、scope、创建/撤销和最后使用时间。设备撤销后 heartbeat 和 ingest 均被拒绝。")
    section(doc, "11.5 审计")
    paragraph(doc, "登录、设备注册与撤销、工作角色分配、项目合并/重分配、事件 tombstone、数据重算、模型调用和策略更新进入追加写 audit_logs。审计记录 actor、动作、资源、范围、结果和时间，不复制业务 payload 与凭据。")

    chapter(doc, 12, "原始事件接收与证据存储模块", "描述 AetherisEvent 合同、幂等接收、不可变证据、删除和替代语义。")
    add_status_strip(doc, "规范事件与原始证据", "Core + Go Server + PostgreSQL", "schema v1 向后兼容", "POST /api/v1/ingest")
    section(doc, "12.1 AetherisEvent 信封")
    paragraph(doc, "所有采集源统一包装为 AetherisEvent。信封把身份、项目、来源、发生时间、内容哈希、脱敏报告、处理许可和 payload 分开，使服务端可以在不理解每种适配器细节的情况下进行校验、幂等和审计。")
    add_table(doc, ["字段组", "字段示例", "用途"], [
        ["身份", "tenant_id、subject_id、device_id、project_id", "由设备凭据和项目注册共同约束。"],
        ["事件", "event_id、schema_version、event_type", "全局幂等键和版本化合同。"],
        ["来源", "source、source_version、session_id、correlation_id", "跨源关联和适配器诊断。"],
        ["时间", "occurred_at、ingested_at", "业务发生时间和服务端接收时间分离。"],
        ["内容", "payload、content_hash、content_refs", "只包含脱敏、授权后的最小投影。"],
        ["治理", "provenance、redaction_report、processing_grants、crypto_mode", "说明来源、脱敏和允许处理范围。"],
    ], widths=[2.8, 7.2, 6.0])
    section(doc, "12.2 接收流程")
    for item in [
        "校验 Device Token 并反查 tenant、subject 和 device。",
        "限制请求体、批次数量和字段长度，按 JSON Schema 校验事件。",
        "检查项目是否属于设备的有效注册范围，或是否为允许的 AI 工具兜底项目。",
        "按事件入库时的工作角色规则固化角色快照。",
        "以 event_id 插入 PostgreSQL；重复 ID 返回 duplicate，不产生第二行。",
        "返回逐事件 accepted/rejected 结果；合法事件接收不等待派生 Worker。",
    ]:
        add_number(doc, item)
    section(doc, "12.3 不可变、删除和替代")
    paragraph(doc, "events 行不更新。对错误归属或纠正内容，客户端创建新事件并设置 supersedes_event_id；查询当前投影时排除被替代版本。用户删除创建 event_tombstones，记录理由与时间并阻止同 event_id 重放。对象正文如未来启用，events 只保存 event_blobs 引用。")
    section(doc, "12.4 数据库索引")
    paragraph(doc, "events 按 tenant + project + time、tenant + device + time、tenant + work_role + time 建立索引，AI 维度另有表达式/维度索引。分页接口必须使用时间和稳定 ID 组合，不允许浏览器一次加载全部历史 JSONB。")
    section(doc, "12.5 失败行为")
    add_table(doc, ["失败类型", "服务端响应", "客户端行为"], [
        ["重复事件", "duplicate", "ack 并从待上传队列移除。"],
        ["字段非法", "rejected + 字段原因", "进入本地 rejected，保留供 Lens 检查，不无限重试。"],
        ["临时 5xx/网络", "请求失败", "保持 claimed 事件，退避后重试。"],
        ["设备凭据失效", "401/403", "注册状态变为 credential_invalid，停止高频上传。"],
        ["项目未授权", "403/422", "保留本地证据并提示项目授权，不绕过门禁。"],
    ], widths=[3.4, 5.1, 7.5])

    chapter(doc, 13, "事件清洗与事实层模块", "说明如何在不修改原始事件的前提下进行命令合并、执行者归因、跨源去重、隔离和版本化重算。")
    add_status_strip(doc, "Forge 清洗事实", "Go Cleaning Worker + PostgreSQL", "规则版本 v2", "clean_event_facts / cleaning_jobs")
    section(doc, "13.1 设计目标")
    paragraph(doc, "清洗事实层解决 PSReadLine 拆行、AI 工具调用与终端历史重复、项目归属修正和残片误计数问题。它不删除原始证据，而是为同一组 source_event_ids 生成可审计的规范事实。")
    section(doc, "13.2 事实模型")
    add_table(doc, ["字段", "说明"], [
        ["fact_id + rule_version", "稳定逻辑事实 ID和清洗规则版本组成主键，允许新旧版本并存。"],
        ["fact_type / activity_type", "ai_interaction、terminal_operation、activity、command_fragment；并映射 AI、终端、IDE、浏览器、版本控制、应用等活动类型。"],
        ["actor_origin / message_role", "human、ai、unknown；user、assistant、system、tool、unknown。"],
        ["quality_state / confidence", "accepted、merged、quarantined；high、medium、low。"],
        ["reason_codes", "explicit_backtick_join、orphan_parameter_join、automation_dynamic 等可解释原因。"],
        ["source_event_ids / canonical_event_id", "完整证据链和权威证据。"],
        ["excluded_from_effectiveness", "命令残片、待确认或策略排除事实不进入指标与 RAG。"],
    ], widths=[5.1, 10.9])
    section(doc, "13.3 命令清洗")
    paragraph(doc, "Core 能获得物理行号和源偏移时先完成 PowerShell 逻辑组装；服务端继续处理历史启发式关联。像“-Recurse -Filter …”这样的参数开头记录被标记 orphan_parameter_fragment，并检查相邻设备、项目、采集顺序和时间窗口。只有拼接后满足 AST/规则完整性才生成 merged 事实。")
    add_code(doc, r'''
原始证据 A: Get-ChildItem "D:\Program Files (x86)\Windows Kits\10\build"
原始证据 B: -Recurse -Filter Microsoft.DriverKit.Build.Tasks.17.0.dll

规范事实: file.search / 递归查找文件
merge_method: inferred_parameter_join
source_event_ids: [A, B]
计数: 1
''')
    section(doc, "13.4 AI 工具与终端去重")
    paragraph(doc, "结构化 ai.tool_call 是权威命令证据。清洗 Worker 使用设备、项目、HMAC 和时间窗口匹配 PSReadLine；匹配后保留两条 events，但只生成一条可计数 terminal_operation，actor_origin=ai。PSReadLine 只作为补充执行证据，避免同一 AI 操作重复进入效能。")
    section(doc, "13.5 隐私规则")
    paragraph(doc, "AI 执行命令只保留命令类型、安全摘要和设备内 HMAC，不保存或展示完整命令、参数、路径、环境变量和 tool arguments。人工/未知终端可以保存完整脱敏规范命令，但必须通过 DLP。设备 HMAC 密钥不上传，防止常见命令字典反查。")
    section(doc, "13.6 重算与数据质量")
    paragraph(doc, "Cleaning Worker 按 tenant 和事件 ID 增量处理，并可按 1 至 90 天范围重算。任务保存处理、合并、隔离和失败数量。Admin Web 数据质量页按原因码、质量状态、设备、项目和日期分页展示，并允许从事实展开原始证据。新规则完成对账后才切换当前版本。")
    section(doc, "13.7 验收")
    for item in [
        "反引号多行与倒序参数样本都形成一条规范事实。",
        "无法可靠合并的残片进入 quarantined/command_fragment，不进入效能与 RAG。",
        "AI tool 与终端匹配后 source_event_ids 同时保留，效能只计一次。",
        "同一重算任务重试不产生重复事实；旧新 rule_version 可并存和回滚。",
        "页面和 API 不出现完整 AI 命令、原始路径或设备 HMAC。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 14, "Work Episode 工作片段模块", "描述如何把同一主体和逻辑项目的活动组织为可追溯、可修订的工作片段。")
    add_status_strip(doc, "Forge 证据组织", "Go Worker + PostgreSQL + 可选 Ollama", "确定性基础已落地", "Work Episode Admin / Local API")
    section(doc, "14.1 产品形态")
    paragraph(doc, "工作片段不是原始事件的替代，而是围绕一个目标组织的解释性投影。服务端生成租户授权范围内的管理投影；Core 根据本机近期缓存生成当前用户与当前设备的个人投影。两者使用相同证据约束，但独立计算。")
    section(doc, "14.2 数据结构")
    add_table(doc, ["对象", "内容", "证据要求"], [
        ["work_episodes", "项目、主体、设备、开始/结束、标题、目标、上下文、结果、置信度、revision。", "片段自身引用参与事件。"],
        ["actions", "用户、AI、终端、IDE、Git、浏览器等安全动作摘要。", "每个动作至少一个事件 ID。"],
        ["decisions", "明确作出的决策及依据。", "缺少证据则不发布。"],
        ["validations", "测试、构建、静态分析、运行健康或用户确认。", "发出命令本身不能证明成功。"],
        ["next_actions", "有明确证据支持的后续事项。", "有序并保留来源。"],
        ["evidence", "section、item key、event ID、引用原因。", "所有正式陈述可回到事件。"],
    ], widths=[3.0, 7.6, 5.4])
    section(doc, "14.3 候选构建")
    for item in [
        "按主体和逻辑项目分组，并保留明确 session_id 和 correlation_id。",
        "相邻活动超过 30 分钟无信号时切分候选片段。",
        "最早有效用户消息或显式任务作为目标候选。",
        "AI 工具、终端、IDE、Git、浏览器和应用事实形成动作。",
        "只有明确成功结果或验证事实才形成 validation；没有证据时保持 needs_review。",
    ]:
        add_number(doc, item)
    section(doc, "14.4 模型增强")
    paragraph(doc, "Ollama 可在已经确定的证据集合上生成严格 JSON 摘要。返回中的每个目标、动作、决策、验证和结果必须引用输入 evidence ID；无效引用或结构错误被丢弃。模型不可用时保留确定性片段，不影响原始事件和其他派生任务。")
    section(doc, "14.5 修订与失效")
    paragraph(doc, "事件替代、tombstone、项目重新分配、授权撤销或规则版本切换会使关联片段过期。重算写新 revision 并更新当前指针，历史 revision 保留。检索、效能和导出只使用当前有效投影。")

    chapter(doc, 15, "个人效能模块", "说明可解释个人指标的计算口径、聚合、权限、页面呈现和禁止性边界。")
    add_status_strip(doc, "Pulse 个人工作模式", "Go 日聚合 + PostgreSQL", "效能定义 v4", "/api/v1/admin/effectiveness")
    section(doc, "15.1 定位")
    paragraph(doc, "个人效能将清洗后的有效事实转换为工作节奏、活动构成、项目分布和覆盖度。它不把事件数量等同于生产力，不评价好坏，不提供跨员工排名、强制分位数、红黄绿绩效灯或综合总分。")
    section(doc, "15.2 时间与去重")
    paragraph(doc, "所有 occurred_at 先转换为 Asia/Shanghai，再向下取整到五分钟桶。只有该桶内存在未排除的清洗事实才计 5 分钟；连续五分钟没有动作的桶不计。一个主体多设备在同一桶内只计一次；同一 AI tool 与终端双证据只按合并事实计一次；OCR 保底与专属适配器同桶时不重复增加活跃时间。")
    add_callout(doc, "活跃时段口径", "active_window_minutes = 有至少一个有效事实的不同五分钟桶数量 × 5。该数值表达活动信号覆盖，不是工时。", "success")
    section(doc, "15.3 指标定义")
    add_table(doc, ["指标", "计算规则", "解释限制"], [
        ["活跃天数", "周期内至少有一个有效事实的自然日数。", "不表示出勤。"],
        ["活跃时段", "主体全局去重的有效五分钟桶。", "不等于工时、在线或加班。"],
        ["工作会话", "相邻有效事实间隔超过 30 分钟则新会话。", "单事件会话按 5 分钟。"],
        ["专注时段", "同项目相邻桶间隔不超过 10 分钟，持续至少 25 分钟。", "不判断主观专注程度。"],
        ["上下文切换", "同会话内相邻事实项目不同计一次。", "高低都不自动解释为好坏。"],
        ["活动构成", "交付、AI、终端、IDE/编码、浏览器、应用、其他的确定性互斥分类。", "各类事件数不能直接相加解释为工时。"],
        ["覆盖度", "covered_days / period_days，并返回来源、设备和最后事件。", "低于 30% 提示数据不足。"],
    ], widths=[2.8, 8.2, 5.0], font_size=8.0)
    section(doc, "15.4 聚合实现")
    paragraph(doc, "subject_effectiveness_daily 保存主体、日期、时区、定义版本、各指标、项目/角色分布、来源计数、设备列表和代表性 evidence_event_ids。后台 Worker 每 5 分钟重算今天和昨天，修复晚到事件；管理员可提交最多 31 天的异步重算，查询最大 90 天。")
    section(doc, "15.5 趋势")
    paragraph(doc, "API 同时读取当前周期和紧邻等长上一周期，输出绝对值、差值和百分比。上一周期为 0 时百分比为 null，不显示无限增长。趋势只陈述变化，例如“活跃时段增加 20%”，不自动表述为效能提高。")
    section(doc, "15.6 页面与模型总结")
    paragraph(doc, "页面提供主体、7/30/90 天、项目筛选，展示 KPI、每日趋势、活动构成、项目/工作角色分布、口径和证据下钻。AI 总结默认不自动运行；用户触发后只发送聚合指标、覆盖度、趋势、项目/角色分布和口径，并固定显示“不作为绩效评价”。Ollama/Dify 不可用时基础页面完整可用。")
    section(doc, "15.7 权限")
    paragraph(doc, "tenant_admin 可读取租户主体；member 只有通过 user_subject_links 显式绑定后才能读取本人；device_ingest 不能读取。analyst/reviewer 默认不授予完整个人效能，避免使用不完整项目范围产生误导结论。接口没有按效能指标排序参数。")

    chapter(doc, 16, "Admin Web 管理后台模块", "描述独立 React/Vite 前端的导航、页面、状态管理、API 边界与交付方式。")
    add_status_strip(doc, "远程管理与分析", "React + TypeScript + Vite", "静态构建由 Go Server 托管", "HttpOnly Cookie / CSRF / OpenAPI Types")
    section(doc, "16.1 交付与登录体验")
    paragraph(doc, "Admin Web 是独立静态前端，开发时通过 npm run dev 和 API proxy 运行，生产 npm run build 后复制到 Go Server 的 web/admin 目录。访问根路径直接进入登录；登录成功进入主页面，右上角提供客户端下载。它不依赖 Go 模板渲染，可后续独立部署到 Nginx 或 CDN。")
    section(doc, "16.2 信息架构")
    add_table(doc, ["一级页面", "主要能力"], [
        ["总览", "设备、主体、事件、健康、最近活动和模型状态摘要。"],
        ["设备与角色", "设备清单、主体关系、工作角色分配。"],
        ["活动记录", "统一展示 AI 协作、AI 工具、终端、IDE、浏览器、版本控制、应用和其他事实。"],
        ["个人效能", "解释性 KPI、趋势、构成、项目/角色分布、覆盖度和证据。"],
        ["项目管理", "逻辑项目、项目位置、冲突和安全显示名。"],
        ["工作片段", "目标、动作、验证、结果、revision 和证据。"],
        ["智能查询", "异步 RAG 查询、进度、直接答案、置信度和可展开引用。"],
        ["采集覆盖", "按设备展示适配器状态、阶段、延迟和安全错误码。"],
        ["数据质量", "清洗版本、合并、残片、隔离、原因码、证据链和重算。"],
        ["浏览器采集", "启用开关、域名白名单、revision 和隐私说明。"],
        ["VS Code 集成", "扩展组件状态、版本、协议与处理建议。"],
        ["审计日志", "管理写操作、资源、结果和时间。"],
    ], widths=[3.6, 12.4], font_size=8.0)
    section(doc, "16.3 活动记录")
    paragraph(doc, "“AI 交互”和“事件”已合并为“活动记录”。页面调用分页聚合接口，不在浏览器加载全部 15,000 条以上历史。筛选包括日期、设备、项目、活动类型和消息角色；列表显示时间、类型、角色和安全摘要，详情抽屉显示脱敏内容、事实 ID、规范事件 ID和源证据。")
    section(doc, "16.4 交互状态")
    paragraph(doc, "所有页面显式处理 loading、empty、unauthorized、server-error 和 retry 状态。耗时操作如清洗重算、效能重算和智能查询返回 job ID并显示进度，避免按钮无反馈或页面假死。详情抽屉文本采用可读颜色和可复制布局，不依赖选中高亮才能看清。")
    section(doc, "16.5 授权边界")
    paragraph(doc, "前端可以根据当前权限隐藏入口并优化体验，但不能成为授权事实源。每个 API 仍由 Go Server 重新认证、授权和限定 tenant/project scope。前端不持有设备 token、模型网关 token、数据库凭据或服务器 root 凭据。")
    section(doc, "16.6 前端质量")
    for item in [
        "TypeScript 类型来自 OpenAPI/共享类型，接口变化通过构建和测试暴露。",
        "页面使用稳定网格和响应式约束，详情抽屉、表格和筛选在桌面与窄屏不重叠。",
        "生产构建使用内容哈希资源，部署后检查 /admin/ 引用的新资源名称和缓存。",
        "关键页面使用组件测试覆盖加载、空、错误、权限、分页和主要操作。",
    ]:
        add_bullet(doc, item)

    chapter(doc, 17, "智能查询与 RAG 模块", "描述本地 RAG 的索引边界、异步查询、模型生成、证据引用、项目公平性和回答契约。")
    add_status_strip(doc, "Nexus 检索与回答", "Go Retrieval + Qdrant + Model Gateway", "生产运行", "/api/v1/admin/rag/*")
    add_figure(doc, diagrams["rag"], "图 17-1  智能查询异步处理流程")
    section(doc, "17.1 检索文档")
    paragraph(doc, "RAG 只索引当前清洗规则版本中 quality_state 为 accepted/merged 且未排除效能的事实。用户消息、AI 工具、人工终端、Git、IDE、浏览器和应用活动可以作为向量锚点；AI 回复按同设备、同项目和时间邻域加入生成上下文，避免每条回复重复生成向量。")
    add_table(doc, ["活动", "索引内容", "明确排除"], [
        ["AI 消息", "已脱敏的用户/助手内容和安全项目标签。", "未授权 cwd、凭据、系统内部字段。"],
        ["AI 工具", "工具名、命令类型和安全摘要。", "完整命令、arguments、input、路径。"],
        ["终端", "人工/未知来源的完整脱敏规范命令。", "命令残片、待确认 automation。"],
        ["Git/IDE/浏览器/应用", "确定性安全摘要和允许字段。", "patch、源代码、截图、完整 OCR 原文。"],
    ], widths=[3.2, 7.2, 5.6])
    section(doc, "17.2 存储分工")
    paragraph(doc, "PostgreSQL 的 retrieval_documents 保存正文、权限维度、内容哈希、规则版本和索引状态；Qdrant collection aetheris_activities_v1 只保存 768 维向量和 tenant、document、device、project、activity_type、time 等最小 payload。检索命中后 Go Server 再回 PostgreSQL 做权限复核。")
    section(doc, "17.3 异步查询")
    paragraph(doc, "POST 查询只验证问题和日期/设备/项目/活动类型范围，返回 202 与 query_id。后台状态依次为 queued、embedding、retrieving、generating、completed 或 failed；Admin Web 轮询并展示阶段。索引和查询共享查询优先门：当前索引批次完成后，交互查询独占模型资源。")
    section(doc, "17.4 回答模式")
    add_table(doc, ["模式", "问题示例", "主回答约束"], [
        ["direct", "有没有、是否支持", "一至两句，通常不超过 80 个中文字符。"],
        ["numeric", "多少、几次、多久", "先给数值和单位，再给一句口径。"],
        ["reason", "为什么、根因是什么", "最多三个直接原因。"],
        ["procedure", "如何做、步骤是什么", "最多五个可执行步骤。"],
        ["analysis", "详细说明、对比实现与证据", "允许较长说明，但仍必须引用证据。"],
    ], widths=[2.5, 5.0, 8.5])
    paragraph(doc, "模型返回严格 JSON：answer、answer_mode、confidence、details 和 citation_numbers。服务端验证模式、长度、内部技术细节词表和引用编号；非法结构最多带错误反馈重试一次，仍失败则生成确定性保底答案。")
    section(doc, "17.5 引用与证据")
    paragraph(doc, "每个 citation 必须来自本次已授权检索结果，并包含 fact_id 或 event_id、项目、时间和安全摘要。页面默认只显示直接答案、置信度和引用数量；展开“查看依据”后显示证据卡片，并可打开活动详情。证据不足时明确回答“当前数据不足以确认”，不编造。")
    section(doc, "17.6 多项目公平性")
    paragraph(doc, "智能查询默认汇总当前租户全部项目的私有过程知识和已认证公共知识，并按主题、项目与来源配额组装上下文，避免单个高频项目完全淹没其他项目。0.4.16 的客户端公平回填确保历史 Codex 会话进入服务端、清洗和索引；项目缺数据时先报告覆盖问题，而不是用无关证据臆测。")
    section(doc, "17.7 运行参数")
    add_table(doc, ["项目", "生产值"], [
        ["回答模型", "qwen3:4b-instruct"],
        ["Embedding 模型", "embeddinggemma，768 维"],
        ["向量库", "Qdrant 1.19.1，Cosine，loopback"],
        ["稳态索引批量", "16"],
        ["模型超时", "Go 与 Model Gateway 内部 300 秒"],
        ["模型并发", "Ollama 最大同时加载 2 个模型；查询优先。"],
    ], widths=[5.2, 10.8])

    chapter(doc, 18, "Model Gateway 模块", "说明 Python 模型适配层为何独立、如何对接 Ollama/Dify，以及输入输出与故障边界。")
    add_status_strip(doc, "模型 Provider 抽象", "独立 Python HTTP Service", "默认 Ollama，Dify 可选", "/internal/v1/generate /internal/v1/embed")
    section(doc, "18.1 采用 Python 的原因")
    paragraph(doc, "模型生态、SDK 和文本处理在 Python 中成熟且变化快，因此 Ollama/Dify 适配独立实现为 Python 服务；稳定的认证、权限、事实、任务和引用仍留在 Go Server。这样既利用 Python 生态，又避免模型框架侵入核心业务和数据库。")
    section(doc, "18.2 模块边界")
    for item in [
        "只监听 loopback 或明确配置的内网地址，并要求内部 Bearer Token。",
        "不连接 PostgreSQL、Qdrant 或 Windows Core，不拥有用户权限和检索范围。",
        "只接收 Go Server 已经授权、脱敏和裁剪的最小输入。",
        "统一处理 provider 选择、超时、重试、响应大小、结构化输出和错误码。",
        "健康接口 /healthz 只表示网关进程；/readyz 同时验证默认 provider 和模型可用。",
    ]:
        add_bullet(doc, item)
    section(doc, "18.3 Provider")
    add_table(doc, ["Provider", "用途", "配置与约束"], [
        ["OllamaProvider", "默认本地生成；个人效能总结、工作片段增强和 RAG 回答。", "调用本机 /api/chat 或生成接口；默认 qwen3:4b-instruct。"],
        ["Ollama Embeddings", "批量生成检索向量。", "调用 /api/embed；模型 embeddinggemma；校验批量和维度。"],
        ["DifyProvider", "可选工作流或应用编排。", "通过可配置 URL、API key 和 app/workflow ID；仍不拥有数据和引用。"],
    ], widths=[3.6, 6.2, 6.2])
    section(doc, "18.4 生成合同")
    paragraph(doc, "请求包含任务名、逻辑模型、最小 context、messages 和可选证据投影。响应必须通过任务对应的 JSON Schema；网关限制输入/输出大小，并将 provider、模型、耗时和稳定错误码返回 Go Server。原始 prompt 和 response 默认不写日志或 model_runs。")
    section(doc, "18.5 故障降级")
    paragraph(doc, "Ollama 不可达、模型缺失、超时或返回非法结构时，网关返回不含敏感正文的错误。Go Server 根据任务选择降级：工作片段保留确定性结果，个人效能仍展示基础指标，RAG 任务失败或使用确定性答案；任何模型故障不影响 ingest。")

    chapter(doc, 19, "数据存储与基础设施模块", "说明 PostgreSQL、Qdrant、对象存储接口、本地 SQLite 与磁盘空间的职责和容量规划。")
    add_status_strip(doc, "持久化与基础设施", "PostgreSQL + Qdrant + SQLite + 接口化对象存储", "生产运行", "SQL / VectorIndex / BlobStore")
    section(doc, "19.1 存储分层")
    add_table(doc, ["存储", "保存内容", "不保存内容"], [
        ["终端 SQLite", "已脱敏待上传事件、checkpoint、项目注册、授权、适配器健康和有限缓存。", "截图、明文 token、无限历史。"],
        ["PostgreSQL", "身份、权限、不可变事件、清洗事实、派生投影、检索正文、任务、引用和审计。", "未脱敏终端内容、模型文件。"],
        ["Qdrant", "活动文档向量和最小过滤 payload。", "正文、权限事实、凭据。"],
        ["对象存储接口", "未来大对象或导出内容引用。", "首版不强制部署；events 不直接内嵌大对象。"],
        ["Ollama 模型目录", "生成与 embedding 模型文件。", "业务事件和用户权限。"],
    ], widths=[3.2, 7.0, 5.8])
    section(doc, "19.2 PostgreSQL 设计")
    paragraph(doc, "PostgreSQL 是生产唯一事务事实源。当前 migration 001 至 019 依次覆盖身份、角色、事件、审计/模型、个人效能、AI 维度、清洗事实、统一活动/RAG、supersession、浏览器策略、项目注册、项目归属、工作片段、适配器健康、设备维度、VS Code 组件状态、VS Code 效能、应用策略和结构化 RAG 答案。")
    paragraph(doc, "所有租户表包含 tenant_id 并启用 RLS；业务 repository 必须显式传入 tenant scope。结构化列承载高频过滤和关联维度，JSONB 用于版本化或低频扩展字段，避免把所有查询都变成 JSONB 全表扫描。")
    section(doc, "19.3 Qdrant")
    paragraph(doc, "Qdrant 独立于 PostgreSQL，符合向量索引接口化和 15 GB 规划。collection 使用 768 维 Cosine；tenant、device、project、activity_type 和时间建立 payload index。Qdrant 只监听 127.0.0.1:6333/6334，当前 Ubuntu 使用 1.19.1 musl 构建以避免 glibc 2.38 依赖。")
    section(doc, "19.4 容量规划")
    add_table(doc, ["区域", "初始预算", "管理策略"], [
        ["PostgreSQL", "25 GB", "索引与表增长监控，备份前检查余量。"],
        ["对象存储", "40 GB", "尚未强依赖；按哈希和保留策略管理对象。"],
        ["向量索引", "15 GB", "按 collection/model version 重建，不混用不同维度。"],
        ["模型与 Dify 数据", "10 GB", "模型清单受控，避免重复模型占用。"],
        ["日志、备份及升级", "10 GB", "日志轮转、14 天备份保留、版本备份及时清理。"],
    ], widths=[4.2, 3.2, 8.6])
    paragraph(doc, "磁盘使用达到 80% 告警，达到 90% 停止非必要导入和历史重算；heartbeat、管理读取和基本 ingest 应优先保持可用。")
    section(doc, "19.5 接口化扩展")
    paragraph(doc, "Go Server 通过 BlobStore 和 VectorIndex 接口依赖对象存储与向量库。禁用实现允许基础系统在没有可选依赖时启动；启用后健康检查和 ready 状态必须分别报告。更换 Qdrant 或接入 S3/MinIO 不改变 AetherisEvent 和 Admin API。")

    chapter(doc, 20, "安全、隐私与数据治理模块", "汇总终端、传输、服务端、模型和管理界面的安全控制与数据生命周期。")
    add_status_strip(doc, "端到端治理", "终端门禁 + TLS + RBAC/RLS + 审计", "强制边界", "DPAPI / DLP / Tombstone / Rule Version")
    section(doc, "20.1 威胁面与信任边界")
    add_table(doc, ["边界", "主要风险", "控制"], [
        ["用户态 Core ↔ LocalSystem Service", "权限提升、命令注入、敏感内容进入服务", "固定协议、PID/SID/Session 校验、无任意路径/参数。"],
        ["Core ↔ Go Server", "设备冒充、重放、路径/凭据泄漏", "Device Token 哈希、事件 ID幂等、HTTPS、本地脱敏和项目授权。"],
        ["Admin Web ↔ Go Server", "会话劫持、CSRF、越权", "HttpOnly/SameSite、CSRF、Authorizer、RLS、审计。"],
        ["Go Server ↔ Model Gateway", "模型看到过量数据、提示泄漏", "内部 Token、最小投影、超时/大小限制、无数据库连接。"],
        ["Go Server ↔ Qdrant", "跨租户检索、向量正文泄漏", "强制 tenant filter、PostgreSQL 二次复核、Qdrant 无正文。"],
    ], widths=[3.8, 5.1, 7.1], font_size=8.0)
    section(doc, "20.2 本地隐私门禁")
    for item in [
        "进程必须已允许，项目必须已授权，适配器必须启用，才能形成内容事件。",
        "浏览器只采集显式白名单域名的 Chrome/Edge 前台页面；非白名单甚至不进入 OCR。",
        "截图只存在内存，OCR/DLP 后立即释放，不写 SQLite、日志或服务端。",
        "源代码正文、完整 diff、剪贴板、按键、终端输出和第三方扩展私有数据不采集。",
        "未知或高风险内容采取失败关闭：标记 needs_review 或阻止上传。",
    ]:
        add_bullet(doc, item)
    section(doc, "20.3 脱敏与 DLP")
    paragraph(doc, "确定性规则识别 API key、Bearer token、私钥、密码、连接字符串、邮箱和用户目录等内容，替换为带类型占位符。content_hash 基于脱敏后的规范 JSON 计算；RedactionReport 只记录规则 ID和替换数量。OCR 原文先通过 DLP，高风险内容首期直接阻断。")
    section(doc, "20.4 凭据与密钥")
    add_table(doc, ["凭据/密钥", "保存位置", "保护方式"], [
        ["Device Token", "终端 device.credential / 服务端 token_hash", "DPAPI CurrentUser + ACL；服务端不保存明文。"],
        ["Local Control Credential", "终端独立 credential", "与设备 token 分离，用于 Lens 写操作和 CSRF。"],
        ["项目身份密钥", "终端 DPAPI 缓存 / 服务端配置", "租户级和设备级用途分离，带版本。"],
        ["命令指纹密钥", "终端 DPAPI", "随机设备内密钥，从不上传。"],
        ["Model Gateway Token", "服务器 0600 环境文件或 secret", "仅内部服务使用，不进入前端和日志。"],
        ["管理员密码", "PostgreSQL 密码哈希", "与 Linux root 密码完全独立；重置撤销旧会话。"],
    ], widths=[4.0, 5.2, 6.8])
    section(doc, "20.5 数据生命周期")
    paragraph(doc, "原始事件作为不可变证据长期按保留策略保存；纠正产生替代事件，删除产生 tombstone。派生事实、项目归属、工作片段、效能和检索文档带规则/定义版本，可重算、对比和回滚。终端离线队列有 512 MB/7 天边界；高风险隔离默认最多 24 小时且不上传。")
    section(doc, "20.6 禁止性要求")
    add_callout(doc, "明确禁止", "不得存储真实 root 密码、设备 Token、完整 AI 命令、未脱敏 OCR、非白名单浏览器内容或源代码正文；不得用模型生成员工排名、黑盒评分或无证据的完成结论。", "warning")

    chapter(doc, 21, "部署、运维、备份与升级模块", "描述生产目录、systemd、自启动、健康检查、备份、升级、回滚和磁盘管理。")
    add_status_strip(doc, "Linux 私有服务器运维", "Ubuntu + systemd", "192.168.78.138 /opt/aetheris", "健康接口 / pg_dump / 版本备份")
    add_figure(doc, diagrams["deployment"], "图 21-1  生产服务器部署拓扑")
    section(doc, "21.1 生产目录")
    add_code(doc, r'''
/opt/aetheris/
  bin/                 aetheris-server, aetheris-migrate, aetheris-admin
  web/admin/           React/Vite 静态构建产物
  config/server.env    0600，服务凭据与运行参数
  models/ollama/       Ollama 模型
  vector/qdrant/       Qdrant 数据
  backups/             PostgreSQL 与发布前备份
  logs/                必要的文件日志；优先 journald
''')
    section(doc, "21.2 systemd 自启动")
    add_table(doc, ["Unit", "监听", "启动策略"], [
        ["postgresql.service", "127.0.0.1:5432 或受控地址", "enabled，数据库先就绪。"],
        ["aetheris-server.service", "0.0.0.0:8080", "enabled，依赖网络和 PostgreSQL。"],
        ["aetheris-model-gateway.service", "127.0.0.1:18081", "enabled，模型网关。"],
        ["ollama.service", "127.0.0.1:11434", "enabled，生成与 embedding。"],
        ["qdrant.service", "127.0.0.1:6333/6334", "enabled，向量索引。"],
        ["aetheris-gateway.service", "旧 8080", "disabled/inactive，已被 Go Server 替代。"],
        ["aetheris-legacy-gateway.service", "人工回滚", "disabled，禁止随系统启动。"],
    ], widths=[5.0, 4.2, 6.8])
    paragraph(doc, "服务器重启后不能只检查进程存在；必须同时检查 is-enabled、is-active、监听端口、/healthz 或 /readyz、pg_isready 以及本次 boot 的 error 日志。")
    section(doc, "21.3 部署流程")
    for item in [
        "只读检查磁盘、目录、现有服务、数据库和监听端口。",
        "生成发布包和校验值，上传到 staging，不直接覆盖运行目录。",
        "备份 PostgreSQL、当前二进制、Admin Web、环境文件权限和客户端安装包。",
        "执行 aetheris-migrate；迁移只增量添加或兼容扩展。",
        "原子切换 Go Server 与 Admin Web，重启相关 unit。",
        "验证健康、登录、客户端下载、设备 heartbeat、事件 ingest、Qdrant 和 Model Gateway ready。",
        "观察后台任务与日志，再清理过期 staging；保留明确回滚版本。",
    ]:
        add_number(doc, item)
    section(doc, "21.4 备份与恢复")
    paragraph(doc, "deploy/backup-postgres.sh 使用 pg_dump custom format，默认保留 14 天。恢复前停止写入并确认目标数据库；恢复后执行 migration 版本检查、RLS/索引校验、/healthz 和业务抽样。Qdrant 可由 PostgreSQL retrieval_documents 和 embedding 模型版本重建，因此不能替代 PostgreSQL 备份。")
    section(doc, "21.5 升级与回滚")
    paragraph(doc, "服务端升级保留上一二进制、web/admin 和配置备份；数据库 migration 必须向前兼容旧二进制的读取窗口，或提供明确回滚路径。新规则和模型索引先并行生成，验收后切换版本指针。客户端默认下载始终指向一个已验收的 Setup，版本化文件和 SHA-256 留存用于回滚。")
    section(doc, "21.6 凭据运维")
    paragraph(doc, "生产密码、Token 和密钥只通过 0600 环境文件、SSH agent 或 secret 机制注入，不写入代码、普通文档、测试和日志。管理员密码重置使用专用 aetheris-admin 命令并撤销旧会话；Linux root 凭据与应用管理员账号无任何关联。")

    chapter(doc, 22, "可观测性、测试与验收模块", "定义运行指标、日志边界、自动化测试、真实环境验收和发布门禁。")
    add_status_strip(doc, "质量与运行保障", "本地状态 + 服务指标 + 自动化/实机测试", "持续执行", "Health / Adapter Health / Jobs / Audit")
    section(doc, "22.1 可观测性分层")
    add_table(doc, ["层级", "观测内容", "禁止内容"], [
        ["Windows Service", "版本、SessionId、Core PID、退出码、恢复次数、稳定错误码。", "用户名、完整 SID、命令行、配置、token、项目路径。"],
        ["Windows Core", "注册/采集/队列状态、适配器健康、事件 ID、计数和耗时。", "命令正文、OCR 文本、URL、payload、凭据。"],
        ["Go Server", "request ID、状态码、延迟、Worker backlog、job ID、规则版本、数量。", "事件 payload、模型 prompt/response、密码。"],
        ["Model/RAG", "provider readiness、模型、耗时、阶段、索引 pending/failed、引用数量。", "未授权正文、内部 token。"],
        ["Admin Web", "加载、空、权限、错误和 job 进度。", "服务端堆栈和敏感配置。"],
    ], widths=[3.1, 7.5, 5.4], font_size=8.0)
    section(doc, "22.2 核心健康检查")
    add_table(doc, ["组件", "检查"], [
        ["Go Server", "GET /healthz；数据库连接、迁移版本和基础依赖。"],
        ["Model Gateway", "/healthz 检查进程；/readyz 检查 Ollama 与默认模型。"],
        ["Ollama", "/api/tags 和 systemctl is-active。"],
        ["Qdrant", "/readyz、collection 维度、points_count 与 payload indexes。"],
        ["PostgreSQL", "pg_isready、连接池 ping、RLS/迁移检查。"],
        ["Core", "服务 ready、heartbeat、队列、适配器快照和服务端设备可见。"],
    ], widths=[4.0, 12.0])
    section(doc, "22.3 测试金字塔")
    for item in [
        "Python 单元测试：项目扫描、身份、DPAPI、命令清洗、适配器、队列、Lens 和 Core 状态机。",
        "Go 单元/集成测试：认证、授权、事件校验、SQL repository、清洗、项目归属、工作片段、效能、Qdrant 和结构化答案。",
        "React 测试：登录、导航、加载/空/错误、活动详情、角色分配、效能、数据质量、RAG、策略和采集覆盖。",
        "原生 C++ 测试：SCM、会话启动、IPC ACL、恢复、Provisioning、项目扫描和 NSIS 插件。",
        "契约测试：OpenAPI、JSON Schema、VS Code 命名管道、Service IPC 和 Model Gateway 协议。",
        "端到端测试：真实 Windows 安装、真实 VS Code、真实 Git/SVN/AI 历史、断网恢复、服务端部署和页面点击。",
    ]:
        add_bullet(doc, item)
    section(doc, "22.4 发布门禁")
    add_table(doc, ["门禁", "通过条件"], [
        ["构建", "Setup/Core/Service/VSIX/Go/Admin Web/Model Gateway 均成功，版本和校验值一致。"],
        ["隐私扫描", "产物、日志、请求和数据库中不存在明文凭据、未授权路径、完整 AI 命令、截图和代码正文。"],
        ["数据一致性", "原始事件、事实、合并、排除、项目归属、检索文档和 Qdrant points 可对账。"],
        ["权限", "跨租户、跨主体、跨项目、member 越权和 device_ingest 读取均被拒绝。"],
        ["恢复", "网络恢复、Core 崩溃、服务器重启、Worker 重跑、规则回滚和部署回滚通过。"],
        ["体验", "登录直达、主页面、客户端下载、活动详情、异步进度和空/错状态可用。"],
    ], widths=[3.3, 12.7])
    section(doc, "22.5 产品级验收矩阵")
    add_table(doc, ["场景", "端到端结果"], [
        ["新终端安装", "用户选目录和项目 → enrollment → heartbeat → 服务 Running → Admin 设备可见。"],
        ["多项目 AI 历史", "公平回填覆盖全部会话文件 → 清洗 → 项目归属 → 索引 → 查询可引用目标项目。"],
        ["拆分命令", "两条原始证据保留 → 一条 merged 事实 → 效能计数一次 → 数据质量可展开原因。"],
        ["浏览器活动", "策略启用 + 白名单 + Chrome/Edge 前台 → OCR/DLP → browser.page_view → 活动/效能/RAG。"],
        ["五分钟无动作", "没有有效事实的桶不计 active_window_minutes；多设备同桶只计一次。"],
        ["服务器重启", "五个正式 unit 自启动，旧 Gateway 不启动，健康与监听全部恢复。"],
        ["模型不可用", "事件和基础页面正常；Episode 使用确定性结果，效能无 AI 总结，查询给出明确失败或降级。"],
    ], widths=[3.7, 12.3], font_size=8.0)

    doc.add_page_break()
    doc.add_heading("附录 A  主要接口", level=1)
    paragraph(doc, "下表列出当前产品的核心接口。具体字段、状态码和 Schema 以 contracts/openapi.yaml 与 schemas/aetheris-event-v1.schema.json 为准。")
    add_table(doc, ["范围", "方法与路径", "用途"], [
        ["设备", "POST /api/v1/device/bootstrap", "用 enrollment code 签发 Device Token。"],
        ["设备", "POST /api/v1/device/heartbeat", "在线状态、服务端事件代际和工作角色投影。"],
        ["设备", "GET /api/v1/device/browser-policy", "拉取浏览器采集策略。"],
        ["设备", "GET /api/v1/device/project-identity-key", "获取版本化租户项目身份密钥。"],
        ["设备", "PUT/POST /api/v1/device/projects", "幂等注册项目位置。"],
        ["设备", "POST /api/v1/device/adapter-health", "上报安全健康快照。"],
        ["采集", "POST /api/v1/ingest", "批量接收不可变 AetherisEvent。"],
        ["认证", "POST /api/v1/auth/login|refresh|logout", "管理员会话。"],
        ["管理", "GET /api/v1/admin/devices|subjects|work-roles", "设备、主体和工作角色。"],
        ["管理", "PUT /api/v1/admin/work-role-assignments", "分配工作角色。"],
        ["活动", "GET /api/v1/admin/activities", "统一活动聚合与分页。"],
        ["数据质量", "GET /api/v1/admin/data-quality/summary|facts", "清洗统计和证据链。"],
        ["数据质量", "POST /api/v1/admin/data-quality/recompute", "异步清洗重算。"],
        ["效能", "GET /api/v1/admin/effectiveness", "个人效能周期结果。"],
        ["效能", "POST /api/v1/admin/effectiveness/recompute|summary", "重算或生成可选总结。"],
        ["项目", "GET /api/v1/admin/projects", "逻辑项目和位置。"],
        ["工作片段", "GET /api/v1/admin/work-episodes[/{id}]", "片段列表、revision 和证据。"],
        ["采集覆盖", "GET /api/v1/admin/adapter-health", "设备适配器健康。"],
        ["策略", "GET/PUT /api/v1/admin/browser-policy", "浏览器域名白名单。"],
        ["RAG", "GET /api/v1/admin/rag/status", "索引与向量状态。"],
        ["RAG", "POST /api/v1/admin/rag/queries", "创建异步查询。"],
        ["RAG", "GET /api/v1/admin/rag/queries/{id}", "读取进度、答案和引用。"],
        ["内部模型", "POST /internal/v1/generate|embed", "结构化生成和批量向量。"],
    ], widths=[2.2, 7.4, 6.4], font_size=7.5)

    doc.add_page_break()
    doc.add_heading("附录 B  核心数据表与事件类型", level=1)
    section(doc, "B.1 核心数据表", level=2)
    add_table(doc, ["领域", "表"], [
        ["身份与权限", "tenants、users、subjects、devices、device_credentials、memberships、permissions、api_sessions"],
        ["角色与项目", "work_roles、work_role_assignments、logical_projects、project_locations、project_attributions、project_backfill_jobs"],
        ["事件与审计", "events、event_tombstones、event_blobs、event_embeddings、audit_logs、model_runs"],
        ["清洗与活动", "clean_event_facts、cleaning_jobs、adapter_health_snapshots"],
        ["工作片段", "work_episodes、work_episode_actions、decisions、validations、next_actions、evidence"],
        ["个人效能", "subject_effectiveness_daily、effectiveness_recompute_jobs、user_subject_links"],
        ["检索与查询", "retrieval_documents、retrieval_query_jobs；向量位于 Qdrant"],
        ["策略", "browser_capture_policies、application_capture_policies"],
    ], widths=[4.0, 12.0])
    section(doc, "B.2 代表性事件类型", level=2)
    add_table(doc, ["活动域", "事件类型示例"], [
        ["版本控制", "git.commit、git.diff、svn.activity"],
        ["终端", "terminal.command"],
        ["AI", "ai.session、ai.message、ai.tool_call"],
        ["IDE", "ide.activity、ide.file_opened、ide.file_edited、ide.file_saved、ide.file_closed、ide.workspace_changed、ide.extension_changed"],
        ["Visual Studio", "visualstudio.activity、构建/调试/测试相关安全事件"],
        ["浏览器", "browser.page_view"],
        ["通用应用", "application.activity"],
        ["设备与诊断", "process.observed、适配器健康通过独立接口上报"],
    ], widths=[4.0, 12.0])

    doc.add_page_break()
    doc.add_heading("附录 C  术语与实现基线", level=1)
    add_table(doc, ["术语", "定义"], [
        ["Core", "Windows 当前用户会话中的采集、脱敏、队列和托盘运行时。"],
        ["Lens", "只在本机回环地址提供的状态、授权和证据控制面。"],
        ["Forge", "清洗事实、项目归属和工作片段处理能力。"],
        ["Nexus", "检索、引用、智能查询和模型编排能力。"],
        ["Pulse", "不含排名与黑盒总分的可解释个人工作模式分析。"],
        ["原始事件", "通过 ingest 保存且不可修改的 AetherisEvent。"],
        ["清洗事实", "由一个或多个原始事件派生、带规则版本和原因码的规范事实。"],
        ["工作片段", "围绕项目和目标组织的有证据活动投影。"],
        ["Enrollment code", "管理员提供的一次性设备注册密钥，不是 Device Token。"],
        ["逻辑项目", "跨设备和克隆目录的安全稳定项目身份。"],
        ["活跃时段", "存在有效活动事实的不同五分钟桶数量乘以五，不等于工时。"],
    ], widths=[4.0, 12.0])
    section(doc, "C.1 当前实现基线", level=2)
    add_table(doc, ["项目", "基线"], [
        ["Windows 客户端", "Aetheris Setup/Core 0.4.16；原生 Windows Service；Local Lens 优先端口 15473。"],
        ["Go Server", "PostgreSQL 14；migration 001-019；对外 8080。"],
        ["清洗", "规则版本 2；原始事件不可变。"],
        ["个人效能", "定义版本 4；时区 Asia/Shanghai。"],
        ["RAG", "Qdrant 1.19.1；aetheris_activities_v1；embeddinggemma 768 维。"],
        ["生成模型", "Ollama qwen3:4b-instruct；Dify 可选。"],
        ["生产目录", "/opt/aetheris；PostgreSQL、Go Server、Model Gateway、Ollama、Qdrant 由 systemd 自启动。"],
    ], widths=[4.0, 12.0])
    section(doc, "C.2 设计来源", level=2)
    paragraph(doc, "本文档由当前代码结构、OpenAPI 合同、PostgreSQL migrations、中文设计规格和运维实施记录汇总而成。发生冲突时，优先级为：已部署实现与自动化测试 → 当前接口契约和 migration → 最新实施记录 → 早期设计规格。后续版本应同步更新本说明书的产品基线和修订记录。")



def main() -> None:
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    diagrams = build_diagrams()
    doc = build_document(diagrams)
    add_remaining_chapters(doc, diagrams)
    doc.save(OUTPUT)
    print(OUTPUT)


if __name__ == "__main__":
    main()
