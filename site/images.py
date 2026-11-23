#!/usr/bin/env python3
"""Makes the example site's icons and link-preview images in site/static/ (site/README.md).

    python3 site/images.py [CHROME]

Run by hand when the design changes, then commit the files it writes; the site build only copies them.
Needs Pillow. The link-preview images need Google Chrome or Chromium (CHROME, default: the macOS app path) for the
text, set in Pretendard when it is installed (else the system sans-serif), so another machine may make slightly
different pixels. The icons are drawn with Pillow alone: the same pixels everywhere, though the file bytes may differ
with the Pillow version. The export lists each image by its SHA-256 (BINARY_OK in scripts/export_public.py), so after
running this, update those hashes. A changed image needs a new file name to reach link previews soon: Facebook,
KakaoTalk and others keep a fetched og:image for a long time.

Design: off-white paper, the "Leeway" wordmark, a three-part bar with the middle part amber, and columns of dots
at the sides. Everything that matters in a preview sits in the centre square, so a square crop still reads.
"""
import subprocess, sys, tempfile
from pathlib import Path
from PIL import Image, ImageDraw

OUT = Path(__file__).resolve().parent / 'static'
PAPER, INK, GREY, AMBER, RUST, SUB = '#f6f4ee', '#1e1f21', '#e4e0d6', '#d9a441', '#a8452f', '#6b6a66'
CHROME = sys.argv[1] if len(sys.argv) > 1 else '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'

# ---------------------------------------------------------------- the icon: an "L" whose foot is the amber bar
# on a 32-unit tile. The SVG is the favicon itself; the PNG and ICO are drawn from the same shapes.
STEM = (9, 6, 15, 23)     # ink stem, x0 y0 x1 y1
FOOT = (9, 20, 24, 26)    # amber foot, a pill (radius 3)
SVG = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">
<rect width="32" height="32" rx="7" fill="{PAPER}"/>
<rect x="{STEM[0]}" y="{STEM[1]}" width="{STEM[2] - STEM[0]}" height="{STEM[3] - STEM[1]}" fill="{INK}"/>
<rect x="{FOOT[0]}" y="{FOOT[1]}" width="{FOOT[2] - FOOT[0]}" height="{FOOT[3] - FOOT[1]}" rx="3" fill="{AMBER}"/>
</svg>
'''


def icon(px, rounded=True):
    """The icon at px pixels, drawn 8 times larger and scaled down (smooth edges)."""
    k = px * 8 / 32
    s = lambda box: tuple(round(v * k) for v in box)
    im = Image.new('RGBA', (px * 8, px * 8), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    d.rounded_rectangle((0, 0, px * 8 - 1, px * 8 - 1), radius=round(7 * k) if rounded else 0, fill=PAPER)
    d.rectangle(s(STEM), fill=INK)
    d.rounded_rectangle(s(FOOT), radius=round(3 * k), fill=AMBER)
    return im.resize((px, px), Image.LANCZOS)


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
    icon(32).save(OUT / 'favicon.ico', sizes=[(16, 16), (32, 32)], append_images=[icon(16)])
    with tempfile.TemporaryDirectory() as tmp:
        for lang in TEXT:
            preview(lang, tmp).save(OUT / f'og-{lang}.png', optimize=True)
    for p in sorted(OUT.iterdir()):
        print(f'{p.name}: {p.stat().st_size} bytes')


if __name__ == '__main__':
    main()
