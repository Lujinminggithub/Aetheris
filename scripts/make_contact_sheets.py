from __future__ import annotations

import sys
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont


def main() -> None:
    folder = Path(sys.argv[1]).resolve()
    pages = sorted(folder.glob("page-*.png"))
    font_path = Path(r"C:\Windows\Fonts\arial.ttf")
    font = ImageFont.truetype(str(font_path), 20) if font_path.exists() else ImageFont.load_default()
    for group_index in range(0, len(pages), 9):
        group = pages[group_index:group_index + 9]
        sheet = Image.new("RGB", (1350, 1920), "#dfe5e5")
        draw = ImageDraw.Draw(sheet)
        for item_index, path in enumerate(group):
            image = Image.open(path).convert("RGB")
            image.thumbnail((420, 570))
            col = item_index % 3
            row = item_index // 3
            x = 20 + col * 445 + (420 - image.width) // 2
            y = 42 + row * 625
            sheet.paste(image, (x, y))
            draw.text((20 + col * 445, 12 + row * 625), path.stem, font=font, fill="#203033")
        target = folder / f"contact-{group_index // 9 + 1:02d}.png"
        sheet.save(target, optimize=True)
        print(target)


if __name__ == "__main__":
    main()
