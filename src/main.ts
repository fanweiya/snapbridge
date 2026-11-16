import { Shot } from "./mygo";

type ToolName = "select" | "rect" | "arrow" | "pen" | "text" | "step" | "highlight" | "mosaic";

interface Point {
  x: number;
  y: number;
}

interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

type Shape =
  | { kind: "rect"; box: Box; color: string; weight: number }
  | { kind: "arrow"; from: Point; to: Point; color: string; weight: number }
  | { kind: "pen"; points: Point[]; color: string; weight: number }
  | { kind: "text"; at: Point; lines: string[]; color: string; size: number }
  | { kind: "step"; at: Point; index: number; color: string }
  | { kind: "highlight"; box: Box; color: string }
  | { kind: "mosaic"; box: Box; block: number };

const $ = <T extends HTMLElement>(id: string): T => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`缺少元素 #${id}`);
  return el as T;
};

const app = $("app");
const bgCanvas = $<HTMLCanvasElement>("bgCanvas");
const selectionBox = $("selectionBox");
const annoCanvas = $<HTMLCanvasElement>("annoCanvas");
const toolbar = $("toolbar");
const sizeBadge = $("sizeBadge");
const textEditor = $<HTMLTextAreaElement>("textEditor");
const magnifier = $("magnifier");
const magCanvas = $<HTMLCanvasElement>("magCanvas");
const magInfo = $("magInfo");
const clipModePill = $("clipModePill");

const dpr = window.devicePixelRatio || 1;

let snapshotWidth = 0;
let snapshotHeight = 0;
let image: HTMLImageElement | null = null;
let selection: Box | null = null;
let shapes: Shape[] = [];
let redoStack: Shape[] = [];
let tool: ToolName = "select";
let strokeColor = "#e53935";
let strokeWeight = 4;
let stepCounter = 0;
let dragMode: "none" | "select" | "move" | "draw" | "handle" | "shape" = "none";
let handleDir = "";
let dragOrigin: Point = { x: 0, y: 0 };
let dragBoxOrigin: Box = { x: 0, y: 0, w: 0, h: 0 };
let selectedShapeIndex = -1;
let dragShapeOrigin: Shape | null = null;
let draft: Shape | null = null;
let pendingText: Point | null = null;
let clipMode = "both";

// 图像物理像素与窗口 CSS 像素的比例。
function ratio(): number {
  return snapshotWidth / window.innerWidth;
}

function toImagePoint(e: MouseEvent): Point {
  const r = ratio();
  return { x: e.clientX * r, y: e.clientY * r };
}

function inside(box: Box, p: Point): boolean {
  return p.x >= box.x && p.x <= box.x + box.w && p.y >= box.y && p.y <= box.y + box.h;
}

function normalize(box: Box): Box {
  const x = box.w < 0 ? box.x + box.w : box.x;
  const y = box.h < 0 ? box.y + box.h : box.y;
  return { x, y, w: Math.abs(box.w), h: Math.abs(box.h) };
}

function clampBox(box: Box): Box {
  const x = Math.max(0, Math.min(box.x, snapshotWidth - 1));
  const y = Math.max(0, Math.min(box.y, snapshotHeight - 1));
  const w = Math.max(1, Math.min(box.w, snapshotWidth - x));
  const h = Math.max(1, Math.min(box.h, snapshotHeight - y));
  return { x, y, w, h };
}

function drawBackground(): void {
  if (!image) return;
  const w = window.innerWidth;
  const h = window.innerHeight;
  bgCanvas.width = Math.round(w * dpr);
  bgCanvas.height = Math.round(h * dpr);
  bgCanvas.style.width = `${w}px`;
  bgCanvas.style.height = `${h}px`;
  const ctx = bgCanvas.getContext("2d");
  if (!ctx) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  ctx.imageSmoothingEnabled = true;
  ctx.imageSmoothingQuality = "high";
  ctx.drawImage(image, 0, 0, w, h);
}

function layoutSelection(): void {
  if (!selection || selection.w < 1 || selection.h < 1) {
    selectionBox.classList.add("hidden");
    toolbar.classList.add("hidden");
    return;
  }
  const r = ratio();
  const css = {
    x: selection.x / r,
    y: selection.y / r,
    w: selection.w / r,
    h: selection.h / r,
  };
  selectionBox.classList.remove("hidden");
  selectionBox.style.left = `${css.x}px`;
  selectionBox.style.top = `${css.y}px`;
  selectionBox.style.width = `${css.w}px`;
  selectionBox.style.height = `${css.h}px`;
  sizeBadge.textContent = `${Math.round(selection.w)} × ${Math.round(selection.h)}`;
  if (css.y < 28) {
    sizeBadge.style.top = "6px";
    sizeBadge.style.left = "6px";
  } else {
    sizeBadge.style.top = "-26px";
    sizeBadge.style.left = "0px";
  }
  magnifier.classList.add("hidden");
  layoutToolbar(css);
  renderAnnotations();
}

function layoutToolbar(box: { x: number; y: number; w: number; h: number }): void {
  toolbar.classList.remove("hidden");
  const tbW = toolbar.offsetWidth;
  const tbH = toolbar.offsetHeight;
  const vw = window.innerWidth;
  const vh = window.innerHeight;

  let left = box.x + box.w - tbW;
  let top = box.y + box.h + 10;

  if (top + tbH > vh - 8) {
    top = box.y - tbH - 10;
  }
  if (top < 8) {
    top = box.y + box.h - tbH - 10;
  }
  if (top < 8) {
    top = 8;
  }
  left = Math.max(8, Math.min(left, vw - tbW - 8));

  toolbar.style.left = `${left}px`;
  toolbar.style.top = `${top}px`;
}

function renderAnnotations(): void {
  if (!selection) return;
  const r = ratio();
  const cssW = Math.max(1, selection.w / r);
  const cssH = Math.max(1, selection.h / r);

  const selCanvas = $<HTMLCanvasElement>("selCanvas");
  selCanvas.width = Math.round(cssW * dpr);
  selCanvas.height = Math.round(cssH * dpr);
  selCanvas.style.width = `${cssW}px`;
  selCanvas.style.height = `${cssH}px`;
  const sctx = selCanvas.getContext("2d");
  if (sctx && image) {
    sctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    sctx.clearRect(0, 0, cssW, cssH);
    sctx.imageSmoothingEnabled = true;
    sctx.imageSmoothingQuality = "high";
    sctx.drawImage(image, selection.x, selection.y, selection.w, selection.h, 0, 0, cssW, cssH);
  }

  annoCanvas.width = Math.round(cssW * dpr);
  annoCanvas.height = Math.round(cssH * dpr);
  annoCanvas.style.width = `${cssW}px`;
  annoCanvas.style.height = `${cssH}px`;

  const ctx = annoCanvas.getContext("2d");
  if (!ctx) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, cssW, cssH);

  // 换算到图像坐标系，让标注坐标与导出坐标完全一致。
  const k = selection.w / cssW;
  ctx.scale(1 / k, 1 / k);
  ctx.translate(-selection.x, -selection.y);

  const scale = ratio();
  for (const s of shapes) paintShape(ctx, s, scale);
  if (draft) paintShape(ctx, draft, scale);
  if (selectedShapeIndex >= 0 && selectedShapeIndex < shapes.length) {
    const sel = shapes[selectedShapeIndex];
    if (sel) {
      const b = getShapeBounds(sel);
      ctx.save();
      ctx.strokeStyle = "rgba(41, 121, 255, 0.95)";
      ctx.lineWidth = 1.5 * scale;
      ctx.setLineDash([4 * scale, 4 * scale]);
      ctx.strokeRect(b.x - 3 * scale, b.y - 3 * scale, b.w + 6 * scale, b.h + 6 * scale);
      ctx.restore();
    }
  }
}

function paintShape(ctx: CanvasRenderingContext2D, s: Shape, scale: number): void {
  ctx.save();
  if (s.kind === "rect") {
    ctx.strokeStyle = s.color;
    ctx.lineWidth = s.weight * scale;
    ctx.lineJoin = "round";
    ctx.strokeRect(s.box.x, s.box.y, s.box.w, s.box.h);
  } else if (s.kind === "arrow") {
    paintArrow(ctx, s.from, s.to, s.color, s.weight * scale);
  } else if (s.kind === "pen") {
    if (s.points.length > 0) {
      ctx.strokeStyle = s.color;
      ctx.lineWidth = s.weight * scale;
      ctx.lineCap = "round";
      ctx.lineJoin = "round";
      ctx.beginPath();
      const first = s.points[0];
      if (first) ctx.moveTo(first.x, first.y);
      for (let i = 1; i < s.points.length; i++) {
        const p = s.points[i];
        if (p) ctx.lineTo(p.x, p.y);
      }
      ctx.stroke();
    }
  } else if (s.kind === "text") {
    const size = s.size * scale;
    ctx.fillStyle = s.color;
    ctx.font = `600 ${size}px -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif`;
    ctx.textBaseline = "top";
    ctx.shadowColor = "rgba(0,0,0,0.55)";
    ctx.shadowBlur = size * 0.25;
    s.lines.forEach((line, i) => {
      ctx.fillText(line, s.at.x, s.at.y + i * size * 1.32);
    });
  } else if (s.kind === "step") {
    const rad = 15 * scale;
    ctx.beginPath();
    ctx.arc(s.at.x, s.at.y, rad, 0, Math.PI * 2);
    ctx.fillStyle = s.color;
    ctx.fill();
    ctx.strokeStyle = "rgba(255,255,255,0.9)";
    ctx.lineWidth = 2 * scale;
    ctx.stroke();
    ctx.fillStyle = "#ffffff";
    ctx.font = `700 ${18 * scale}px -apple-system, "Segoe UI", sans-serif`;
    ctx.textAlign = "center";
    ctx.textBaseline = "middle";
    ctx.fillText(String(s.index), s.at.x, s.at.y + scale);
  } else if (s.kind === "highlight") {
    ctx.fillStyle = s.color;
    ctx.globalAlpha = 0.38;
    ctx.fillRect(s.box.x, s.box.y, s.box.w, s.box.h);
  } else if (s.kind === "mosaic") {
    paintMosaic(ctx, s.box, s.block);
  }
  ctx.restore();
}

function paintArrow(ctx: CanvasRenderingContext2D, from: Point, to: Point, color: string, width: number): void {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const len = Math.hypot(dx, dy);
  if (len < 1) return;
  const head = Math.max(width * 3.2, 12);
  const angle = Math.atan2(dy, dx);
  const bodyEnd = {
    x: to.x - Math.cos(angle) * head * 0.72,
    y: to.y - Math.sin(angle) * head * 0.72,
  };
  ctx.strokeStyle = color;
  ctx.fillStyle = color;
  ctx.lineWidth = width;
  ctx.lineCap = "round";
  ctx.beginPath();
  ctx.moveTo(from.x, from.y);
  ctx.lineTo(bodyEnd.x, bodyEnd.y);
  ctx.stroke();

  ctx.beginPath();
  ctx.moveTo(to.x, to.y);
  ctx.lineTo(to.x - Math.cos(angle - 0.42) * head, to.y - Math.sin(angle - 0.42) * head);
  ctx.lineTo(to.x - Math.cos(angle + 0.42) * head, to.y - Math.sin(angle + 0.42) * head);
  ctx.closePath();
  ctx.fill();
}

function paintMosaic(ctx: CanvasRenderingContext2D, box: Box, block: number): void {
  if (!image || box.w < 2 || box.h < 2) return;
  const bw = Math.max(1, Math.round(box.w / block));
  const bh = Math.max(1, Math.round(box.h / block));
  const tmp = document.createElement("canvas");
  tmp.width = bw;
  tmp.height = bh;
  const tctx = tmp.getContext("2d");
  if (!tctx) return;
  tctx.imageSmoothingEnabled = true;
  tctx.drawImage(image, box.x, box.y, box.w, box.h, 0, 0, bw, bh);
  const prev = ctx.imageSmoothingEnabled;
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(tmp, 0, 0, bw, bh, box.x, box.y, box.w, box.h);
  ctx.imageSmoothingEnabled = prev;
}

function getShapeBounds(s: Shape): Box {
  if (s.kind === "rect" || s.kind === "highlight" || s.kind === "mosaic") {
    return { ...s.box };
  } else if (s.kind === "arrow") {
    const x = Math.min(s.from.x, s.to.x);
    const y = Math.min(s.from.y, s.to.y);
    return { x, y, w: Math.max(1, Math.abs(s.to.x - s.from.x)), h: Math.max(1, Math.abs(s.to.y - s.from.y)) };
  } else if (s.kind === "pen") {
    let minX = Infinity;
    let minY = Infinity;
    let maxX = -Infinity;
    let maxY = -Infinity;
    for (const pt of s.points) {
      if (pt.x < minX) minX = pt.x;
      if (pt.y < minY) minY = pt.y;
      if (pt.x > maxX) maxX = pt.x;
      if (pt.y > maxY) maxY = pt.y;
    }
    if (!Number.isFinite(minX)) return { x: 0, y: 0, w: 0, h: 0 };
    return { x: minX, y: minY, w: Math.max(1, maxX - minX), h: Math.max(1, maxY - minY) };
  } else if (s.kind === "step") {
    return { x: s.at.x - 16, y: s.at.y - 16, w: 32, h: 32 };
  } else if (s.kind === "text") {
    const maxLen = Math.max(...s.lines.map((l) => l.length), 1);
    return { x: s.at.x, y: s.at.y, w: maxLen * s.size * 0.75, h: s.lines.length * s.size * 1.35 };
  }
  return { x: 0, y: 0, w: 0, h: 0 };
}

function distanceToSegment(p: Point, a: Point, b: Point): number {
  const l2 = (b.x - a.x) ** 2 + (b.y - a.y) ** 2;
  if (l2 === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  let t = ((p.x - a.x) * (b.x - a.x) + (p.y - a.y) * (b.y - a.y)) / l2;
  t = Math.max(0, Math.min(1, t));
  return Math.hypot(p.x - (a.x + t * (b.x - a.x)), p.y - (a.y + t * (b.y - a.y)));
}

function hitTestShape(p: Point): number {
  for (let i = shapes.length - 1; i >= 0; i--) {
    const s = shapes[i];
    if (!s) continue;
    if (s.kind === "rect" || s.kind === "highlight" || s.kind === "mosaic") {
      const tol = 8;
      if (
        p.x >= s.box.x - tol &&
        p.x <= s.box.x + s.box.w + tol &&
        p.y >= s.box.y - tol &&
        p.y <= s.box.y + s.box.h + tol
      ) {
        return i;
      }
    } else if (s.kind === "arrow") {
      if (distanceToSegment(p, s.from, s.to) <= 12) {
        return i;
      }
    } else if (s.kind === "pen") {
      for (let j = 0; j < s.points.length - 1; j++) {
        const pt1 = s.points[j];
        const pt2 = s.points[j + 1];
        if (pt1 && pt2 && distanceToSegment(p, pt1, pt2) <= 10) {
          return i;
        }
      }
    } else if (s.kind === "step") {
      if (Math.hypot(p.x - s.at.x, p.y - s.at.y) <= 20) {
        return i;
      }
    } else if (s.kind === "text") {
      const maxLen = Math.max(...s.lines.map((l) => l.length), 1);
      const w = maxLen * s.size * 0.75;
      const h = s.lines.length * s.size * 1.35;
      if (p.x >= s.at.x - 4 && p.x <= s.at.x + w + 4 && p.y >= s.at.y - 4 && p.y <= s.at.y + h + 4) {
        return i;
      }
    }
  }
  return -1;
}

function cloneShape(s: Shape): Shape {
  if (s.kind === "rect" || s.kind === "highlight" || s.kind === "mosaic") {
    return { ...s, box: { ...s.box } };
  } else if (s.kind === "arrow") {
    return { ...s, from: { ...s.from }, to: { ...s.to } };
  } else if (s.kind === "pen") {
    return { ...s, points: s.points.map((pt) => ({ ...pt })) };
  } else if (s.kind === "text") {
    return { ...s, at: { ...s.at }, lines: [...s.lines] };
  } else if (s.kind === "step") {
    return { ...s, at: { ...s.at } };
  }
  return s;
}

function translateShape(s: Shape, origin: Shape, dx: number, dy: number): void {
  if (s.kind === "rect" || s.kind === "highlight" || s.kind === "mosaic") {
    if (origin.kind === s.kind) {
      s.box.x = origin.box.x + dx;
      s.box.y = origin.box.y + dy;
    }
  } else if (s.kind === "arrow") {
    if (origin.kind === "arrow") {
      s.from.x = origin.from.x + dx;
      s.from.y = origin.from.y + dy;
      s.to.x = origin.to.x + dx;
      s.to.y = origin.to.y + dy;
    }
  } else if (s.kind === "pen") {
    if (origin.kind === "pen") {
      for (let i = 0; i < s.points.length; i++) {
        const opt = origin.points[i];
        const pt = s.points[i];
        if (opt && pt) {
          pt.x = opt.x + dx;
          pt.y = opt.y + dy;
        }
      }
    }
  } else if (s.kind === "text" || s.kind === "step") {
    if (origin.kind === s.kind) {
      s.at.x = origin.at.x + dx;
      s.at.y = origin.at.y + dy;
    }
  }
}

function beginSelect(p: Point): void {
  dragMode = "select";
  dragOrigin = p;
  selection = { x: p.x, y: p.y, w: 0, h: 0 };
  shapes = [];
  redoStack = [];
  stepCounter = 0;
  draft = null;
  selectedShapeIndex = -1;
  dragShapeOrigin = null;
  layoutSelection();
}

function beginMove(p: Point): void {
  dragMode = "move";
  dragOrigin = p;
  if (selection) dragBoxOrigin = { ...selection };
}

function beginDraw(p: Point): void {
  if (!selection) return;
  if (tool === "text") {
    openTextEditor(p);
    return;
  }
  if (tool === "step") {
    stepCounter += 1;
    shapes.push({ kind: "step", at: p, index: stepCounter, color: strokeColor });
    renderAnnotations();
    return;
  }
  dragMode = "draw";
  dragOrigin = p;
  if (tool === "rect") {
    draft = { kind: "rect", box: { x: p.x, y: p.y, w: 0, h: 0 }, color: strokeColor, weight: strokeWeight };
  } else if (tool === "arrow") {
    draft = { kind: "arrow", from: p, to: p, color: strokeColor, weight: strokeWeight };
  } else if (tool === "pen") {
    draft = { kind: "pen", points: [p], color: strokeColor, weight: strokeWeight };
  } else if (tool === "highlight") {
    draft = { kind: "highlight", box: { x: p.x, y: p.y, w: 0, h: 0 }, color: strokeColor };
  } else if (tool === "mosaic") {
    draft = { kind: "mosaic", box: { x: p.x, y: p.y, w: 0, h: 0 }, block: Math.max(6, 8 * ratio()) };
  }
}

function updateDrag(p: Point, shift: boolean): void {
  if (dragMode === "select") {
    const raw = normalize({ x: dragOrigin.x, y: dragOrigin.y, w: p.x - dragOrigin.x, h: p.y - dragOrigin.y });
    selection = clampBox(raw);
    layoutSelection();
    return;
  }
  if (dragMode === "move" && selection) {
    const dx = p.x - dragOrigin.x;
    const dy = p.y - dragOrigin.y;
    selection = clampBox({ ...dragBoxOrigin, x: dragBoxOrigin.x + dx, y: dragBoxOrigin.y + dy });
    layoutSelection();
    return;
  }
  if (dragMode === "shape" && selectedShapeIndex >= 0 && dragShapeOrigin) {
    const s = shapes[selectedShapeIndex];
    if (s) {
      const dx = p.x - dragOrigin.x;
      const dy = p.y - dragOrigin.y;
      translateShape(s, dragShapeOrigin, dx, dy);
      renderAnnotations();
    }
    return;
  }
  if (dragMode === "handle" && selection) {
    const b = { ...dragBoxOrigin };
    if (handleDir.includes("w")) {
      const nx = Math.min(p.x, b.x + b.w - 4);
      b.w = b.x + b.w - nx;
      b.x = nx;
    }
    if (handleDir.includes("e")) {
      b.w = Math.max(4, p.x - b.x);
    }
    if (handleDir.includes("n")) {
      const ny = Math.min(p.y, b.y + b.h - 4);
      b.h = b.y + b.h - ny;
      b.y = ny;
    }
    if (handleDir.includes("s")) {
      b.h = Math.max(4, p.y - b.y);
    }
    selection = clampBox(b);
    layoutSelection();
    return;
  }
  if (dragMode === "draw" && draft) {
    if (draft.kind === "rect" || draft.kind === "highlight" || draft.kind === "mosaic") {
      draft.box = normalize({ x: dragOrigin.x, y: dragOrigin.y, w: p.x - dragOrigin.x, h: p.y - dragOrigin.y });
      if (shift && draft.kind === "rect") {
        const side = Math.max(Math.abs(draft.box.w), Math.abs(draft.box.h));
        draft.box.w = side;
        draft.box.h = side;
      }
    } else if (draft.kind === "arrow") {
      draft.to = p;
    } else if (draft.kind === "pen") {
      draft.points.push(p);
    }
    renderAnnotations();
  }
}

function endDrag(): void {
  if (dragMode === "select" && selection) {
    if (selection.w < 6 || selection.h < 6) {
      selection = null;
    }
  }
  if (dragMode === "draw" && draft) {
    const keep =
      (draft.kind === "pen" && draft.points.length > 1) ||
      ((draft.kind === "rect" || draft.kind === "highlight" || draft.kind === "mosaic") &&
        draft.box.w > 3 &&
        draft.box.h > 3) ||
      (draft.kind === "arrow" && Math.hypot(draft.to.x - draft.from.x, draft.to.y - draft.from.y) > 6);
    if (keep) {
      shapes.push(draft);
      redoStack = [];
    }
    draft = null;
  }
  if (dragMode === "shape") {
    dragShapeOrigin = null;
  }
  dragMode = "none";
  layoutSelection();
}

function openTextEditor(p: Point): void {
  pendingText = p;
  const r = ratio();
  textEditor.classList.remove("hidden");
  textEditor.style.left = `${p.x / r}px`;
  textEditor.style.top = `${p.y / r}px`;
  textEditor.style.color = strokeColor;
  textEditor.value = "";
  textEditor.focus();
  autoGrow();
}

function autoGrow(): void {
  textEditor.style.height = "auto";
  textEditor.style.height = `${textEditor.scrollHeight}px`;
  textEditor.style.width = `${Math.max(160, textEditor.scrollWidth + 12)}px`;
}

function commitText(): void {
  if (!pendingText) return;
  const value = textEditor.value.trim();
  textEditor.classList.add("hidden");
  if (value) {
    shapes.push({
      kind: "text",
      at: pendingText,
      lines: value.split("\n"),
      color: strokeColor,
      size: 16,
    });
    redoStack = [];
  }
  pendingText = null;
  textEditor.value = "";
  renderAnnotations();
}

function cancelText(): void {
  pendingText = null;
  textEditor.value = "";
  textEditor.classList.add("hidden");
}

function compose(): string {
  if (!image) throw new Error("截图尚未加载");
  const box = clampBox(selection ?? { x: 0, y: 0, w: snapshotWidth, h: snapshotHeight });
  const out = document.createElement("canvas");
  out.width = Math.round(box.w);
  out.height = Math.round(box.h);
  const ctx = out.getContext("2d");
  if (!ctx) throw new Error("无法创建导出画布");
  ctx.drawImage(image, box.x, box.y, box.w, box.h, 0, 0, box.w, box.h);
  ctx.translate(-box.x, -box.y);
  const scale = ratio();
  for (const s of shapes) paintShape(ctx, s, scale);
  return out.toDataURL("image/png");
}

let busy = false;

async function submit(copy: boolean, save: boolean, mode?: string): Promise<void> {
  if (busy) return;
  busy = true;
  try {
    if (pendingText) commitText();
    const png = compose();
    await Shot.commit({ png, save, copy, clipMode: mode ?? "" });
  } catch (err) {
    busy = false;
    console.error(err);
    const message = err instanceof Error ? err.message : String(err);
    alert(`提交失败: ${message}`);
    return;
  }
  busy = false;
}

function setTool(next: ToolName): void {
  tool = next;
  selectedShapeIndex = -1;
  for (const btn of Array.from(document.querySelectorAll<HTMLElement>(".tool-btn"))) {
    const active = btn.dataset.tool === next;
    btn.classList.toggle("active", active);
    btn.setAttribute("aria-pressed", active ? "true" : "false");
  }
  renderAnnotations();
}

function undo(): void {
  selectedShapeIndex = -1;
  const last = shapes.pop();
  if (last) {
    redoStack.push(last);
    if (last.kind === "step") stepCounter = Math.max(0, stepCounter - 1);
    renderAnnotations();
  }
}

function redo(): void {
  selectedShapeIndex = -1;
  const last = redoStack.pop();
  if (last) {
    shapes.push(last);
    if (last.kind === "step") stepCounter += 1;
    renderAnnotations();
  }
}

function updateMagnifier(e: MouseEvent): void {
  if (!image || selection || dragMode !== "none") {
    magnifier.classList.add("hidden");
    return;
  }
  const p = toImagePoint(e);
  const size = 96;
  const zoom = 6;
  const ctx = magCanvas.getContext("2d");
  if (!ctx) return;
  ctx.imageSmoothingEnabled = false;
  ctx.clearRect(0, 0, size, size);
  ctx.drawImage(image, p.x - size / zoom / 2, p.y - size / zoom / 2, size / zoom, size / zoom, 0, 0, size, size);
  ctx.strokeStyle = "rgba(41,121,255,0.9)";
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(size / 2, 0);
  ctx.lineTo(size / 2, size);
  ctx.moveTo(0, size / 2);
  ctx.lineTo(size, size / 2);
  ctx.stroke();

  magInfo.textContent = `${Math.round(p.x)}, ${Math.round(p.y)}`;
  magnifier.classList.remove("hidden");
  const magW = 100;
  const magH = 130;
  let left = e.clientX + 24;
  let top = e.clientY + 24;
  if (left + magW > window.innerWidth - 8) {
    left = e.clientX - magW - 16;
  }
  if (top + magH > window.innerHeight - 8) {
    top = e.clientY - magH - 16;
  }
  magnifier.style.left = `${Math.max(8, left)}px`;
  magnifier.style.top = `${Math.max(8, top)}px`;
}

function bindEvents(): void {
  app.addEventListener("pointerdown", (e) => {
    if (e.button !== 0) return;
    magnifier.classList.add("hidden");
    const target = e.target as HTMLElement;
    if (target.closest("#toolbar") || target.closest("#hintBar") || target.closest("#textEditor")) return;
    const handle = target.closest<HTMLElement>(".handle");
    if (handle) {
      dragMode = "handle";
      handleDir = handle.dataset.dir ?? "";
      dragOrigin = toImagePoint(e);
      if (selection) dragBoxOrigin = { ...selection };
      e.preventDefault();
      return;
    }
    if (pendingText) commitText();
    const p = toImagePoint(e);
    if (!selection || !inside(selection, p)) {
      beginSelect(p);
      return;
    }
    if (tool === "select") {
      const hit = hitTestShape(p);
      if (hit !== -1) {
        selectedShapeIndex = hit;
        const targetShape = shapes[hit];
        if (targetShape) {
          dragMode = "shape";
          dragOrigin = p;
          dragShapeOrigin = cloneShape(targetShape);
          renderAnnotations();
          return;
        }
      }
      selectedShapeIndex = -1;
      renderAnnotations();
      beginMove(p);
      return;
    }
    beginDraw(p);
  });

  window.addEventListener("pointermove", (e) => {
    if (!selection && dragMode === "none") updateMagnifier(e);
    else magnifier.classList.add("hidden");
    if (dragMode === "none") return;
    updateDrag(toImagePoint(e), e.shiftKey);
  });

  window.addEventListener("pointerleave", () => {
    magnifier.classList.add("hidden");
  });

  window.addEventListener("pointerup", () => {
    if (dragMode === "none") return;
    if (dragMode === "handle") dragMode = "none";
    else endDrag();
  });

  app.addEventListener("dblclick", (e) => {
    const target = e.target as HTMLElement;
    if (target.closest("#toolbar") || target.closest("#hintBar")) return;
    if (tool === "select") void submit(true, false);
  });

  window.addEventListener("keydown", (e) => {
    if (e.target === textEditor) return;
    const key = e.key;
    if (key === "Escape") {
      e.preventDefault();
      void Shot.cancel();
      return;
    }
    if ((key === "Delete" || key === "Backspace") && selectedShapeIndex >= 0 && selectedShapeIndex < shapes.length) {
      e.preventDefault();
      const removed = shapes.splice(selectedShapeIndex, 1)[0];
      if (removed) {
        redoStack.push(removed);
      }
      selectedShapeIndex = -1;
      renderAnnotations();
      return;
    }
    if (key === "Enter" && !e.ctrlKey) {
      e.preventDefault();
      void submit(true, false);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && key.toLowerCase() === "c") {
      e.preventDefault();
      void submit(true, false, e.shiftKey ? "path" : undefined);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && key.toLowerCase() === "s") {
      e.preventDefault();
      void submit(true, true);
      return;
    }
    if ((e.ctrlKey || e.metaKey) && key.toLowerCase() === "z") {
      e.preventDefault();
      undo();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && key.toLowerCase() === "y") {
      e.preventDefault();
      redo();
      return;
    }
    if ((e.ctrlKey || e.metaKey) && key.toLowerCase() === "a") {
      e.preventDefault();
      selection = { x: 0, y: 0, w: snapshotWidth, h: snapshotHeight };
      layoutSelection();
      return;
    }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    switch (key.toLowerCase()) {
      case "v":
        setTool("select");
        break;
      case "r":
        setTool("rect");
        break;
      case "a":
        setTool("arrow");
        break;
      case "f":
        setTool("pen");
        break;
      case "t":
        setTool("text");
        break;
      case "i":
        setTool("step");
        break;
      case "h":
        setTool("highlight");
        break;
      case "b":
        setTool("mosaic");
        break;
      case "delete":
      case "backspace":
        undo();
        break;
    }
  });

  for (const btn of Array.from(document.querySelectorAll<HTMLElement>(".tool-btn"))) {
    btn.addEventListener("click", () => setTool((btn.dataset.tool ?? "select") as ToolName));
  }
  for (const dot of Array.from(document.querySelectorAll<HTMLElement>(".color-dot"))) {
    dot.addEventListener("click", () => {
      strokeColor = dot.dataset.color ?? "#e53935";
      for (const d of Array.from(document.querySelectorAll<HTMLElement>(".color-dot"))) {
        const active = d === dot;
        d.classList.toggle("active", active);
        d.setAttribute("aria-pressed", active ? "true" : "false");
      }
    });
  }
  for (const sb of Array.from(document.querySelectorAll<HTMLElement>(".stroke-btn"))) {
    sb.addEventListener("click", () => {
      strokeWeight = Number(sb.dataset.size ?? "4");
      for (const s of Array.from(document.querySelectorAll<HTMLElement>(".stroke-btn"))) {
        const active = s === sb;
        s.classList.toggle("active", active);
        s.setAttribute("aria-pressed", active ? "true" : "false");
      }
    });
  }

  $("btnUndo").addEventListener("click", undo);
  $("btnSave").addEventListener("click", () => void submit(true, true));
  $("btnCopyPath").addEventListener("click", () => void submit(true, false, "path"));
  $("btnOk").addEventListener("click", () => void submit(true, false));
  $("btnClose").addEventListener("click", () => void Shot.cancel());

  textEditor.addEventListener("blur", () => {
    if (pendingText) commitText();
  });
  textEditor.addEventListener("input", autoGrow);
  textEditor.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      commitText();
    } else if (e.key === "Escape") {
      e.preventDefault();
      cancelText();
    }
  });

  clipModePill.addEventListener("click", async () => {
    const order = ["both", "image", "path"];
    const idx = order.indexOf(clipMode);
    const next = order[(idx + 1) % order.length] ?? "both";
    clipMode = next;
    await Shot.setClipMode(next);
    paintClipMode();
  });

  window.addEventListener("resize", () => {
    drawBackground();
    layoutSelection();
  });
}

function paintClipMode(): void {
  const label = clipMode === "both" ? "模式: 图+路径" : clipMode === "image" ? "模式: 只要图片" : "模式: 只要路径";
  clipModePill.textContent = label;
}

async function init(): Promise<void> {
  const snap = await Shot.snapshot();
  snapshotWidth = snap.width;
  snapshotHeight = snap.height;
  clipMode = snap.clipMode || "both";
  paintClipMode();

  const img = new Image();
  img.src = snap.imageUrl;
  await img.decode();
  image = img;

  drawBackground();

  if (snap.preset && snap.preset.length === 4) {
    const [x, y, w, h] = snap.preset;
    if (x !== undefined && y !== undefined && w !== undefined && h !== undefined) {
      selection = clampBox({ x, y, w, h });
    }
  }
  bindEvents();
  layoutSelection();
  setTool("select");
  await Shot.ready();
}

void init().catch(async (err) => {
  console.error(err);
  await Shot.ready().catch(() => undefined);
  alert(`加载截图失败: ${err instanceof Error ? err.message : String(err)}`);
});
