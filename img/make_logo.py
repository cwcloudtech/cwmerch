"""Build cwmerch's transparent logos from ai-gen/assets/logo.jpeg: python3 img/make_logo.py"""
import os
import numpy as np
HERE = os.path.dirname(os.path.abspath(__file__))
from PIL import Image
from scipy import ndimage as ndi
src = Image.open(os.path.join(HERE, '..', 'ai-gen', 'assets', 'logo.jpeg')).convert('RGB')
a = np.asarray(src).astype(float)
mn, mx = a.min(2), a.max(2)
whiteish = (mn >= 226) & ((mx - mn) <= 20)
lab, _ = ndi.label(whiteish)
border = set(np.unique(np.concatenate([lab[0], lab[-1], lab[:, 0], lab[:, -1]]))) - {0}
holes = set()
for i, sl in enumerate(ndi.find_objects(lab), 1):
    if sl is None or i in border:
        continue
    # enclosed white pockets that are background: the cart's grid cells and the
    # gap between a tentacle and the basket (the eyes stay opaque)
    if sl[1].start > 1500 or sl[0].start > 1000:
        holes.add(i)
bg = np.isin(lab, list(border | holes))
# small holes enclosed by the shape stay opaque; only the outside background goes
dist = ndi.distance_transform_edt(~bg)  # distance of foreground px to background
alpha = np.ones(mn.shape)
alpha[bg] = 0.0
band = (~bg) & (dist <= 3)
soft = np.clip((255 - mn) / (255 - 175), 0, 1)
alpha[band] = soft[band]
# un-mix the white from the soft edge so it doesn't halo on dark backgrounds
rgb = a.copy()
m = band & (alpha > 0.02)
for c in range(3):
    ch = rgb[..., c]
    ch[m] = np.clip((a[..., c][m] - (1 - alpha[m]) * 255) / alpha[m], 0, 255)
# 1px smoothing of alpha edges
alpha = ndi.gaussian_filter(alpha, 0.6) * (dist > 0) + 0 * bg
alpha[(~bg) & (dist > 3)] = 1.0
out = np.dstack([rgb, alpha * 255]).astype(np.uint8)
img = Image.fromarray(out, 'RGBA')
bbox = img.getbbox()
img = img.crop(bbox)
pad = 24
canvas = Image.new('RGBA', (img.width + 2 * pad, img.height + 2 * pad), (0, 0, 0, 0))
canvas.alpha_composite(img, (pad, pad))
canvas.save(os.path.join(HERE, 'cwmerch-logo.png'), optimize=True)
# mascot only: the largest opaque component, plus the pieces inside its box
opaque = np.asarray(canvas)[..., 3] > 20
olab, _ = ndi.label(opaque)
sizes = ndi.sum(opaque, olab, range(1, olab.max() + 1))
main = int(np.argmax(sizes)) + 1
sl = ndi.find_objects(olab)[main - 1]
x0, x1, y0, y1 = sl[1].start, sl[1].stop, sl[0].start, sl[0].stop
keep = np.zeros_like(opaque)
for i, s2 in enumerate(ndi.find_objects(olab), 1):
    if s2 is not None and s2[1].start >= x0 and s2[1].stop <= x1 and s2[0].start >= y0 and s2[0].stop <= y1:
        keep |= olab == i
arr = np.asarray(canvas).copy()
arr[..., 3] = np.where(keep, arr[..., 3], 0)
mascot = Image.fromarray(arr, 'RGBA').crop((x0, y0, x1, y1))
x = x1
side = max(mascot.size) + 40
sq = Image.new('RGBA', (side, side), (0, 0, 0, 0))
sq.alpha_composite(mascot, ((side - mascot.width) // 2, (side - mascot.height) // 2))
for size, name in [(1024, 'cwmerch-icon-1024.png'), (512, 'cwmerch-icon-512.png'), (180, 'apple-touch-icon.png')]:
    sq.resize((size, size), Image.LANCZOS).save(os.path.join(HERE, name), optimize=True)
sq.resize((64, 64), Image.LANCZOS).save(os.path.join(HERE, 'favicon.ico'), sizes=[(16, 16), (32, 32), (48, 48), (64, 64)])
canvas.resize((canvas.width // 2, canvas.height // 2), Image.LANCZOS).save(os.path.join(HERE, 'cwmerch-logo-small.png'), optimize=True)
print(canvas.size, x, sq.size)

# SVG wrappers (the artwork is raster: the SVG embeds the transparent PNG)
import base64, io
for png, svg, label in [('cwmerch-logo.png', 'cwmerch-logo.svg', 'cwmerch logo'), ('cwmerch-icon-512.png', 'cwmerch-icon.svg', 'cwmerch')]:
    im = Image.open(os.path.join(HERE, png))
    buf = io.BytesIO()
    im.save(buf, 'PNG', optimize=True)
    b64 = base64.b64encode(buf.getvalue()).decode()
    with open(os.path.join(HERE, svg), 'w') as f:
        f.write(f'<svg xmlns="http://www.w3.org/2000/svg" width="{im.width}" height="{im.height}" viewBox="0 0 {im.width} {im.height}" role="img" aria-label="{label}"><title>{label}</title><image width="{im.width}" height="{im.height}" href="data:image/png;base64,{b64}"/></svg>\n')
