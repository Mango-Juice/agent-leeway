#!/usr/bin/env python3
"""Makes the example site's icons and link-preview images in site/static/ (site/README.md).

    python3 site/images.py [CHROME] [--icons]     # --icons: only the icons, the previews are left as they are

Run by hand when the design changes, then commit the files it writes; the site build only copies them.
Needs Pillow. The link-preview images need Google Chrome or Chromium (CHROME, default: the macOS app path) for the
text, set in Pretendard when it is installed (else the system sans-serif), so another machine may make slightly
different pixels. The icons are drawn with Pillow alone: the same pixels everywhere, though the file bytes may differ
with the Pillow version. The export lists each image by its SHA-256 (BINARY_OK in scripts/export_public.py), so after
running this, update those hashes. A changed image needs a new file name to reach link previews soon: Facebook,
KakaoTalk and others keep a fetched og:image for a long time.

Icon: leeway in the nautical sense. On a navy tile, a dotted line is the intended course and an amber curve the path
actually taken, pushed off it, both from one start dot. At 16 px the dotted line is left out.
Previews: off-white paper, the "Leeway" wordmark, a three-part bar with the middle part amber, and columns of dots at
the sides. Everything that matters in a preview sits in the centre square, so a square crop still reads.
"""
import subprocess, sys, tempfile
from pathlib import Path
from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parent / 'static'
PAPER, INK, GREY, AMBER, RUST, SUB = '#f6f4ee', '#1e1f21', '#e4e0d6', '#d9a441', '#a8452f', '#6b6a66'
ARGS = [a for a in sys.argv[1:] if not a.startswith('--')]
if set(sys.argv[1:]) - set(ARGS) - {'--icons'}:
    sys.exit('usage: python3 site/images.py [CHROME] [--icons]')
CHROME = ARGS[0] if ARGS else '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

# ---------------------------------------------------------------- the icon: leeway in the nautical sense
# The dotted line is the intended course, the amber curve the path actually taken, pushed off course; the dot is where
# both start. 64 units on a navy tile. The SVG is the favicon itself; the PNG and ICO are drawn from the same geometry.
NAVY, CURVE = '#15304d', '#e0a63f'
ICO16_COURSE = False  # at 16 px the dotted line is a grey smudge: left out (judged from the rendered image)
START, COURSE_END = (14, 48), (50, 16)
BEZIER = ((14, 48), (26, 44), (40, 42), (52, 34))
SVG = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
<rect width="64" height="64" rx="14" fill="{NAVY}"/>
<path d="M14 48 L50 16" stroke="{PAPER}" stroke-opacity=".45" stroke-width="4" stroke-linecap="round" stroke-dasharray="1 8"/>
<path d="M14 48 C26 44 40 42 52 34" stroke="{CURVE}" stroke-width="6" stroke-linecap="round" fill="none"/>
<circle cx="14" cy="48" r="5" fill="{PAPER}"/>
</svg>
'''


def icon(px, rounded=True, course=True):
    """The icon at px pixels, drawn 16 times larger and scaled down (smooth edges). rounded=False fills the whole
    square (the phone rounds it); course=False leaves out the dotted line, which turns to a smudge at 16 px."""
    S = px * 16
    k = S / 64
    im = Image.new('RGBA', (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle((0, 0, S - 1, S - 1), radius=round(14 * k) if rounded else 0, fill=NAVY)

    def stroke(dr, points, width, fill):  # a polyline with round caps and joins
        r = width * k / 2
        pts = [(x * k, y * k) for x, y in points]
        dr.line(pts, fill=fill, width=round(width * k))
        for x, y in pts:
            dr.ellipse((x - r, y - r, x + r, y + r), fill=fill)

    if course:
        # dash 1, gap 8 along the line, width 4, round caps: each dash is a capsule of length 1 every 9 units,
        # painted white at 45% over the navy
        layer = Image.new('RGBA', (S, S), (0, 0, 0, 0))
        dl = ImageDraw.Draw(layer)
        (x0, y0), (x1, y1) = START, COURSE_END
        length = ((x1 - x0) ** 2 + (y1 - y0) ** 2) ** .5
        ux, uy = (x1 - x0) / length, (y1 - y0) / length
        t = 0.0
        while t <= length:
            e = min(t + 1, length)
            stroke(dl, [(x0 + ux * t, y0 + uy * t), (x0 + ux * e, y0 + uy * e)], 4, PAPER)
            t += 9
        layer.putalpha(layer.getchannel('A').point(lambda a: round(a * .45)))
        im.alpha_composite(layer)
    n = 200
    curve = []
    for i in range(n + 1):
        u = i / n
        (ax, ay), (bx, by), (cx, cy), (ex, ey) = BEZIER
        w = ((1 - u) ** 3, 3 * (1 - u) ** 2 * u, 3 * (1 - u) * u * u, u ** 3)
        curve.append((w[0] * ax + w[1] * bx + w[2] * cx + w[3] * ex, w[0] * ay + w[1] * by + w[2] * cy + w[3] * ey))
    stroke(d, curve, 6, CURVE)
    r = 5 * k
    d.ellipse((START[0] * k - r, START[1] * k - r, START[0] * k + r, START[1] * k + r), fill=PAPER)
    return im.convert('RGBa').reduce(16).convert('RGBA')  # premultiplied: each pixel the average of its 16 x 16 block (area coverage, as a browser draws an SVG)


# ---------------------------------------------------------------- the link-preview image, 1200 x 630
W, H = 1200, 630
TEXT = {
    'en': ('A habit check-up for<br>AI coding agents', 'en'),
    'ko': ('AI 코딩 에이전트<br>사용 습관 점검', 'ko'),
}


def dots(x0, seed):
    """A column block of dots, mostly grey, some rust and amber; the same pattern every run."""
    out, n = [], seed
    for y in range(57, H - 30, 38):
        for x in range(x0, x0 + 5 * 38, 38):
            n = (n * 1103515245 + 12345) % 2 ** 31
            r = n / 2 ** 31
            c = RUST if r < 0.16 else AMBER if r < 0.24 else GREY
            out.append(f'<circle cx="{x}" cy="{y}" r="8" fill="{c}"/>')
    return ''.join(out)


def preview_html(lang):
    sub, hl = TEXT[lang]
    side = f'<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" style="position:absolute;inset:0">{dots(46, 7)}{dots(1002, 11)}</svg>'
    return f'''<!doctype html><html lang="{hl}"><meta charset="utf-8"><style>
*{{box-sizing:border-box;margin:0;padding:0}}
html,body{{width:{W}px;height:{H}px;overflow:hidden}}
body{{background:{PAPER};font-family:Pretendard,"Apple SD Gothic Neo","Noto Sans KR",system-ui,sans-serif;color:{INK};position:relative;-webkit-font-smoothing:antialiased;word-break:keep-all}}
.c{{position:absolute;left:285px;top:0;width:630px;height:{H}px;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center}}
.k{{font-size:30px;font-weight:600;color:{SUB};letter-spacing:.01em}}
.w{{font-size:140px;font-weight:800;letter-spacing:-.04em;line-height:1;margin:14px 0 26px}}
.s{{font-size:42px;font-weight:700;line-height:1.3;letter-spacing:-.02em}}
.bar{{display:flex;gap:8px;margin-top:40px}}
.bar i{{display:block;width:126px;height:20px;border-radius:10px;background:{GREY}}}
.bar i.on{{background:{AMBER}}}
</style>{side}<div class="c"><div class="k">Claude Code, Codex</div><div class="w">Leeway</div><div class="s">{sub}</div><div class="bar"><i></i><i class="on"></i><i></i></div></div></html>'''


def preview(lang, tmp):
    h = Path(tmp) / f'og-{lang}.html'
    shot = Path(tmp) / f'og-{lang}.png'
    h.write_text(preview_html(lang), encoding='utf-8')
    subprocess.run([CHROME, '--headless=new', '--hide-scrollbars', '--force-device-scale-factor=1', f'--window-size={W},{H}',
                    f'--screenshot={shot}', h.as_uri()], check=True, capture_output=True)
    im = Image.open(shot).convert('RGB')
    assert im.size == (W, H), im.size
    # a palette of 96 colours keeps the file small; the image is flat colour and anti-aliased text
    return im.quantize(colors=96, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE)


def main():
    OUT.mkdir(exist_ok=True)
    (OUT / 'favicon.svg').write_text(SVG, encoding='utf-8')
    icon(180, rounded=False).convert('RGB').save(OUT / 'apple-touch-icon.png', optimize=True)  # the phone rounds it
    icon(32).save(OUT / 'favicon.ico', sizes=[(16, 16), (32, 32)], append_images=[icon(16, course=ICO16_COURSE)])
    if '--icons' not in sys.argv:
        with tempfile.TemporaryDirectory() as tmp:
            for lang in TEXT:
                preview(lang, tmp).save(OUT / f'og-{lang}.png', optimize=True)
    for p in sorted(OUT.iterdir()):
        print(f'{p.name}: {p.stat().st_size} bytes')


if __name__ == '__main__':
    main()
