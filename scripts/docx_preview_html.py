from __future__ import annotations

import base64
import html
import sys
from pathlib import Path

from docx import Document
from docx.oxml.ns import qn
from docx.table import Table
from docx.text.paragraph import Paragraph


def iter_blocks(document):
    body = document.element.body
    for child in body.iterchildren():
        if child.tag == qn("w:p"):
            yield Paragraph(child, document)
        elif child.tag == qn("w:tbl"):
            yield Table(child, document)


def paragraph_html(paragraph: Paragraph, document: Document) -> str:
    style = paragraph.style.name if paragraph.style else ""
    text = html.escape(paragraph.text)
    images = []
    for blip in paragraph._p.xpath(".//a:blip"):
        rid = blip.get(qn("r:embed"))
        if not rid:
            continue
        part = document.part.related_parts.get(rid)
        if not part:
            continue
        content_type = getattr(part, "content_type", "image/png")
        encoded = base64.b64encode(part.blob).decode("ascii")
        images.append(f'<img src="data:{content_type};base64,{encoded}">')

    page_break = bool(paragraph._p.xpath(".//w:br[@w:type='page']"))
    if page_break:
        return '<div class="page-break"></div>'
    if style == "Title":
        return f"<h1 class='cover-title'>{text}</h1>" + "".join(images)
    if style.startswith("Heading 1"):
        return f"<h1>{text}</h1>" + "".join(images)
    if style.startswith("Heading 2"):
        return f"<h2>{text}</h2>" + "".join(images)
    if style.startswith("Heading 3"):
        return f"<h3>{text}</h3>" + "".join(images)
    if style == "Caption CN":
        return f"<p class='caption'>{text}</p>"
    if style.startswith("List Bullet"):
        return f"<p class='bullet'>• {text}</p>"
    if style.startswith("List Number"):
        return f"<p class='number'>{text}</p>"
    image_html = "".join(images)
    if image_html and not text:
        return f"<div class='figure'>{image_html}</div>"
    if not text and not image_html:
        return "<p class='spacer'></p>"
    return f"<p>{text}</p>{image_html}"


def table_html(table: Table) -> str:
    rows = []
    for row_index, row in enumerate(table.rows):
        cells = []
        tag = "th" if row_index == 0 else "td"
        for cell in row.cells:
            value = "<br>".join(html.escape(p.text) for p in cell.paragraphs if p.text)
            cells.append(f"<{tag}>{value}</{tag}>")
        rows.append("<tr>" + "".join(cells) + "</tr>")
    return "<table>" + "".join(rows) + "</table>"


def main() -> None:
    source = Path(sys.argv[1]).resolve()
    target = Path(sys.argv[2]).resolve()
    document = Document(source)
    blocks = []
    for block in iter_blocks(document):
        if isinstance(block, Paragraph):
            blocks.append(paragraph_html(block, document))
        else:
            blocks.append(table_html(block))
    css = """
@page { size: A4; margin: 18mm 20mm; }
* { box-sizing: border-box; }
body { margin: 0; font-family: "Microsoft YaHei", "SimSun", sans-serif; color: #203033; font-size: 9.4pt; line-height: 1.5; }
h1 { color: #176b68; font-size: 18pt; margin: 0 0 12pt; page-break-before: always; page-break-after: avoid; }
h1.cover-title { color: #203033; font-size: 30pt; text-align: center; margin-top: 35mm; page-break-before: auto; }
h2 { color: #203033; font-size: 13pt; margin: 12pt 0 5pt; page-break-after: avoid; }
h3 { color: #356a8a; font-size: 10.5pt; margin: 9pt 0 4pt; page-break-after: avoid; }
p { margin: 0 0 5pt; orphans: 3; widows: 3; }
.bullet { padding-left: 5mm; text-indent: -3mm; margin-bottom: 2pt; }
.number { padding-left: 5mm; margin-bottom: 2pt; }
.caption { text-align: center; color: #526063; font-size: 8.4pt; font-style: italic; margin-bottom: 8pt; }
.spacer { min-height: 5pt; }
.page-break { page-break-after: always; }
.figure { text-align: center; page-break-inside: avoid; }
img { max-width: 100%; max-height: 135mm; object-fit: contain; }
table { width: 100%; border-collapse: collapse; margin: 5pt 0 8pt; page-break-inside: auto; font-size: 8.2pt; }
tr { page-break-inside: avoid; }
th { background: #176b68; color: white; font-weight: 700; }
th, td { border: 0.5pt solid #d9e0e0; padding: 4pt 5pt; vertical-align: middle; }
tr:nth-child(even) td { background: #f8fafa; }
"""
    result = "<!doctype html><html><head><meta charset='utf-8'><style>" + css + "</style></head><body>" + "\n".join(blocks) + "</body></html>"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(result, encoding="utf-8")
    print(target)


if __name__ == "__main__":
    main()
