"""生成 SnapBridge 的应用图标与托盘图标。

设计：深色圆角底 + 四角取景框 + 中心对焦点，
取景框代表框选截图，中心圆点代表捕捉到的内容。
"""

import math
import os
import struct
import sys

import numpy as np
from PIL import Image, ImageChops, ImageDraw, ImageFilter

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
RES = os.path.join(ROOT, "resources")

BASE = 1024          # 设计坐标系边长
SS = 4               # 超采样倍数
BG_TOP = (0x16, 0x22, 0x38)
BG_BOTTOM = (0x07, 0x0C, 0x17)
ACCENT = (0x3F, 0xB6, 0xF5)
ACCENT_DEEP = (0x4F, 0x8E, 0xF7)
WHITE = (0xFF, 0xFF, 0xFF)


def lerp(a, b, t):
    return tuple(int(round(a[i] + (b[i] - a[i]) * t)) for i in range(3))


def gradient(size, top, bottom, diagonal=True):
    y = np.linspace(0.0, 1.0, size, dtype=np.float32)[:, None]
    x = np.linspace(0.0, 1.0, size, dtype=np.float32)[None, :]
    t = np.clip((x + y) / 2.0, 0.0, 1.0) if diagonal else np.repeat(y, size, axis=1)
    out = np.zeros((size, size, 3), dtype=np.uint8)
    for c in range(3):
        out[:, :, c] = (top[c] + (bottom[c] - top[c]) * t).astype(np.uint8)
    return Image.fromarray(out, "RGB")


def rounded_mask(size, radius, ss=1):
    m = Image.new("L", (size, size), 0)
    d = ImageDraw.Draw(m)
    d.rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return m


def thick_segment(draw, p0, p1, width, fill):
    """带圆头的粗线段。"""
    draw.line([p0, p1], fill=fill, width=width, joint="curve")
    r = width / 2.0
    for p in (p0, p1):
        draw.ellipse([p[0] - r, p[1] - r, p[0] + r, p[1] + r], fill=fill)


def draw_brackets(draw, box, arm, width, fill):
    l, t, r, b = box
    w = width
    thick_segment(draw, (l, t + arm), (l, t), w, fill)
    thick_segment(draw, (l, t), (l + arm, t), w, fill)
    thick_segment(draw, (r, t + arm), (r, t), w, fill)
    thick_segment(draw, (r, t), (r - arm, t), w, fill)
    thick_segment(draw, (l, b - arm), (l, b), w, fill)
    thick_segment(draw, (l, b), (l + arm, b), w, fill)
    thick_segment(draw, (r, b - arm), (r, b), w, fill)
    thick_segment(draw, (r, b), (r - arm, b), w, fill)


def render_app(size):
    """应用图标：满幅圆角方块，用于 exe 与商店列表。"""
    n = BASE * SS
    k = lambda v: v * SS  # 设计坐标 -> 超采样坐标

    # 底色
    bg = gradient(n, BG_TOP, BG_BOTTOM)
    mask = rounded_mask(n, k(210))
    canvas = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    canvas.paste(bg, (0, 0), mask)

    # 顶部柔光，制造一点纵深
    glow = Image.new("L", (n, n), 0)
    ImageDraw.Draw(glow).ellipse([k(-320), k(-720), k(1344), k(300)], fill=46)
    glow = glow.filter(ImageFilter.GaussianBlur(k(80))).point(lambda v: int(v * 0.6))
    glow = ImageChops.multiply(glow, mask)
    canvas.paste(Image.new("RGBA", (n, n), (0x9F, 0xD2, 0xFF, 255)), (0, 0), glow)

    layer = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)

    # 取景框
    draw_brackets(d, (k(186), k(186), k(838), k(838)), k(252), k(92), ACCENT + (255,))

    # 中心对焦点：外环 + 实心圆
    cx = cy = k(512)
    ring_r, ring_w = k(212), k(22)
    d.ellipse(
        [cx - ring_r, cy - ring_r, cx + ring_r, cy + ring_r],
        outline=ACCENT + (120,), width=ring_w,
    )
    dot_r = k(122)
    d.ellipse([cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r], fill=WHITE + (255,))

    canvas = Image.alpha_composite(canvas, layer)
    canvas = Image.alpha_composite(canvas, layer)
    return canvas.resize((size, size), Image.Resampling.LANCZOS)


def render_tray(size):
    """托盘图标：深色圆角底 + 加宽的取景框，16px 下仍可辨认。

    托盘图标不能只画线条：Windows 11 的任务栏默认是浅色，
    纯白透明底的图形在浅色背景上等于隐形，所以这里铺一层深色底板。
    """
    n = BASE * SS
    k = lambda v: v * SS
    bg = gradient(n, BG_TOP, BG_BOTTOM)
    mask = rounded_mask(n, k(268))
    canvas = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    canvas.paste(bg, (0, 0), mask)

    layer = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    draw_brackets(d, (k(158), k(158), k(866), k(866)), k(262), k(112), ACCENT + (255,))
    cx = cy = k(512)
    dot_r = k(146)
    d.ellipse([cx - dot_r, cy - dot_r, cx + dot_r, cy + dot_r], fill=WHITE + (255,))

    canvas = Image.alpha_composite(canvas, layer)
    return canvas.resize((size, size), Image.Resampling.LANCZOS)


def save_ico(img, path, sizes):
    """写入含多尺寸 PNG 载荷的 .ico。"""
    frames = []
    for s in sizes:
        frames.append((s, img.resize((s, s), Image.Resampling.LANCZOS)))

    header = struct.pack("<HHH", 0, 1, len(frames))
    entries = b""
    blobs = b""
    offset = 6 + 16 * len(frames)
    for s, frame in frames:
        import io

        buf = io.BytesIO()
        frame.save(buf, format="PNG", optimize=True)
        data = buf.getvalue()
        dim = 0 if s >= 256 else s
        entries += struct.pack("<BBBBHHII", dim, dim, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
        blobs += data

    with open(path, "wb") as f:
        f.write(header + entries + blobs)


def main():
    os.makedirs(RES, exist_ok=True)

    app = render_app(1024)
    app.save(os.path.join(RES, "icon.png"))
    save_ico(app, os.path.join(RES, "icon.ico"), [16, 24, 32, 48, 64, 128, 256])

    tray = render_tray(64)
    tray.save(os.path.join(RES, "tray-64.png"))
    tray.resize((32, 32), Image.Resampling.LANCZOS).save(os.path.join(RES, "tray-32.png"))

    # 预览图：上排是应用图标的各尺寸，下排两条模拟任务栏，
    # 分别在浅色和深色背景上核对托盘图标是否可见。
    preview = Image.new("RGBA", (760, 520), (245, 245, 247, 255))
    x = 24
    for s in (16, 24, 32, 48, 64, 128, 256):
        tile = app.resize((s, s), Image.Resampling.LANCZOS)
        preview.alpha_composite(tile, (x, 20 + (256 - s) // 2))
        x += s + 24

    for band_y, band_bg in ((300, (238, 238, 242, 255)), (410, (32, 32, 36, 255))):
        preview.alpha_composite(Image.new("RGBA", (760, 100), band_bg), (0, band_y))
        x = 24
        for s in (16, 24, 32, 48):
            preview.alpha_composite(tray.resize((s, s), Image.Resampling.LANCZOS), (x, band_y + (100 - s) // 2))
            x += s + 40
    preview.save(os.path.join(RES, "icon-preview.png"))

    print("icon.png 1024")
    print("icon.ico", [16, 24, 32, 48, 64, 128, 256])
    print("tray-64.png / tray-32.png")
    print("icon-preview.png")


if __name__ == "__main__":
    sys.exit(main())
