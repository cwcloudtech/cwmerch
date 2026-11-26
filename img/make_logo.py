"""Rebuild the goocommerce logo: the CWCloud octopus (octopus-base.png, from
cwclock) with its clock replaced by a shopping basket (logo-basket.svg).

macOS only (renders the SVG with Quick Look); needs Pillow and numpy.
    cd img && python3 make_logo.py
The basket is rendered on white and on black, and its transparency recovered
from the difference, because Quick Look always paints a background.
"""
import os, subprocess, tempfile
import numpy as np
from PIL import Image, ImageDraw

HERE = os.path.dirname(os.path.abspath(__file__))
S = 2  # supersampling


def render(svg, out_dir):
    subprocess.run(["qlmanage", "-t", "-s", "1502", "-o", out_dir, svg],
                   check=True, capture_output=True)
    return os.path.join(out_dir, os.path.basename(svg) + ".png")


def basket_rgba(tmp):
    src = open(os.path.join(HERE, "logo-basket.svg")).read()
    white = os.path.join(tmp, "white.svg")
    black = os.path.join(tmp, "black.svg")
    open(white, "w").write(src)
    open(black, "w").write(src.replace("<defs>", '<rect width="751" height="751" fill="#000"/><defs>', 1))
    w = np.asarray(Image.open(render(white, tmp)).convert("RGB")).astype(float)
    k = np.asarray(Image.open(render(black, tmp)).convert("RGB")).astype(float)
    a = np.clip(1 - (w - k).mean(axis=2) / 255.0, 0, 1)
    rgb = np.where(a[..., None] > 0.001, k / np.maximum(a[..., None], 1e-3), 0)
    return Image.fromarray(np.dstack([np.clip(rgb, 0, 255), a * 255]).astype("uint8"), "RGBA")


def main():
    base = Image.open(os.path.join(HERE, "octopus-base.png")).convert("RGBA")
    big = base.resize((base.width * S, base.height * S), Image.LANCZOS)
    with tempfile.TemporaryDirectory() as tmp:
        out = big.copy()
        out.alpha_composite(basket_rgba(tmp).crop((0, 0, big.width, big.height)))

    def relayer(poly):  # bring a gripping tentacle back in front of the basket
        mask = Image.new("L", big.size, 0)
        ImageDraw.Draw(mask).polygon([(x * S, y * S) for x, y in poly], fill=255)
        out.paste(big, (0, 0), mask)

    relayer([(460, 486), (478, 476), (500, 494), (508, 530), (507, 566), (495, 602),
             (456, 614), (438, 606), (452, 580), (462, 548), (464, 512)])
    relayer([(282, 515), (332, 512), (348, 518), (350, 546), (338, 560), (300, 566), (282, 560)])
    logo = out.resize(base.size, Image.LANCZOS)
    logo.save(os.path.join(HERE, "goocommerce-logo.png"))
    square = Image.new("RGBA", (751, 751), (0, 0, 0, 0))
    square.alpha_composite(logo, ((751 - logo.width) // 2, 0))
    for size, name in [(1024, "goocommerce-logo-1024.png"), (512, "goocommerce-logo-512.png"),
                       (180, "apple-touch-icon.png")]:
        square.resize((size, size), Image.LANCZOS).save(os.path.join(HERE, name))
    square.resize((48, 48), Image.LANCZOS).save(os.path.join(HERE, "favicon.ico"),
                                               sizes=[(16, 16), (32, 32), (48, 48)])


if __name__ == "__main__":
    main()
