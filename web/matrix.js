// Matrix effects: ambient rain, digital-rain screen wipes and glyph decryption.
(function () {
  "use strict";
  const GLYPHS = "ｦｱｳｴｵｶｷｹｺｻｼｽｾｿﾀﾂﾃﾅﾆﾇﾈﾊﾋﾎﾏﾐﾑﾒﾓﾔﾕﾗﾘﾜ0123456789Z:・.=*+-<>¦|";
  const pick = () => GLYPHS[(Math.random() * GLYPHS.length) | 0];
  const mq = window.matchMedia("(prefers-reduced-motion: reduce)");
  let forcedReduced = false;
  const reduced = () => forcedReduced || mq.matches;

  // A rain field drawn on a canvas.
  function Rain(canvas, opts) {
    const ctx = canvas.getContext("2d");
    const size = opts.size || 18;
    let cols = [], w = 0, h = 0, dpr = 1;
    function resize() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      w = canvas.clientWidth; h = canvas.clientHeight;
      canvas.width = Math.max(1, w * dpr); canvas.height = Math.max(1, h * dpr);
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      const n = Math.ceil(w / size);
      cols = Array.from({ length: n }, () => ({ y: -Math.random() * h / size * (opts.spread || 1), v: 0.35 + Math.random() * 0.75 }));
      ctx.fillStyle = "#000500"; ctx.fillRect(0, 0, w, h);
    }
    function step(fade, density) {
      ctx.fillStyle = `rgba(0,5,0,${fade})`;
      ctx.fillRect(0, 0, w, h);
      ctx.font = `${size}px "Cascadia Mono", Consolas, monospace`;
      ctx.textBaseline = "top";
      for (let i = 0; i < cols.length; i++) {
        const c = cols[i];
        if (Math.random() > density) continue;
        const x = i * size, y = c.y * size;
        ctx.fillStyle = "#c8ffd4";
        ctx.fillText(pick(), x, y);
        ctx.fillStyle = "#00ff41";
        ctx.fillText(pick(), x, y - size);
        c.y += c.v;
        if (y > h && Math.random() > 0.96) c.y = -Math.random() * 8;
      }
    }
    resize();
    return { resize, step, ctx, get w() { return w; }, get h() { return h; } };
  }

  // Ambient rain behind the library (very faint; off with reduced motion or when hidden).
  let ambient = null, ambientOn = false, ambientRaf = 0, last = 0;
  function ambientLoop(t) {
    if (!ambientOn) return;
    if (t - last > 70) { ambient.step(0.12, 0.9); last = t; }
    ambientRaf = requestAnimationFrame(ambientLoop);
  }
  function setAmbient(on) {
    const c = document.getElementById("ambient");
    if (!ambient) ambient = Rain(c, { size: 16, spread: 1 });
    on = on && !reduced();
    c.style.display = on ? "block" : "none";
    if (on && !ambientOn) { ambientOn = true; ambientRaf = requestAnimationFrame(ambientLoop); }
    if (!on) { ambientOn = false; cancelAnimationFrame(ambientRaf); }
  }
  window.addEventListener("resize", () => ambient && ambient.resize());

  // Decrypt: scramble the element's text, then resolve it left to right.
  function decrypt(el, ms) {
    if (!el) return;
    const isInput = el.tagName === "INPUT";
    const target = isInput ? el.value : el.textContent;
    if (!target || reduced()) return;
    const dur = ms || 450, start = performance.now();
    function frame(t) {
      const k = Math.min(1, (t - start) / dur);
      const n = Math.floor(target.length * k);
      let s = target.slice(0, n);
      for (let i = n; i < target.length; i++) s += target[i] === " " ? " " : pick();
      if (isInput) el.value = s; else el.textContent = s;
      if (k < 1) requestAnimationFrame(frame);
      else if (isInput) el.value = target; else el.textContent = target;
    }
    requestAnimationFrame(frame);
  }

  // Wipe: rain pours down over the screen, swap() runs under full cover, then the rain drains away.
  let wiping = false;
  function wipe(swap) {
    const c = document.getElementById("wipe");
    if (reduced() || wiping) {
      swap();
      if (!reduced()) return Promise.resolve();
      document.getElementById("app").animate([{ opacity: 0 }, { opacity: 1 }], { duration: 120 });
      return Promise.resolve();
    }
    wiping = true;
    c.style.display = "block";
    c.style.opacity = "1";
    const r = Rain(c, { size: 20, spread: 0.15 });
    const ctx = r.ctx;
    const t0 = performance.now(), cover = 360, drain = 420;
    let swapped = false;
    return new Promise((resolve) => {
      function frame(t) {
        const e = t - t0;
        if (e < cover) {
          // curtain: black descends behind the leading glyphs
          const k = e / cover;
          r.step(0.0, 1);
          ctx.fillStyle = "rgba(0,5,0,0.55)";
          ctx.fillRect(0, 0, r.w, r.h * k * 0.92);
        } else {
          if (!swapped) {
            ctx.fillStyle = "#000500"; ctx.fillRect(0, 0, r.w, r.h);
            swap(); swapped = true;
            document.querySelectorAll(".screen:not([hidden]) [data-decrypt]").forEach((el) => decrypt(el, 420));
          }
          const k = Math.min(1, (e - cover) / drain);
          r.step(0.25, 0.7);
          c.style.opacity = String(1 - k);
          if (k >= 1) {
            c.style.display = "none"; wiping = false; resolve(); return;
          }
        }
        requestAnimationFrame(frame);
      }
      requestAnimationFrame(frame);
    });
  }

  // Scrambled digit for the countdown.
  function scramble(el, text, ms) {
    if (reduced()) { el.textContent = text; return; }
    const start = performance.now();
    (function f(t) {
      const k = (t - start) / (ms || 220);
      el.textContent = k < 1 ? pick() : text;
      if (k < 1) requestAnimationFrame(f);
    })(start);
  }

  window.Matrix = {
    wipe, decrypt, setAmbient, scramble,
    setReduced(v) { forcedReduced = v; if (v) setAmbient(false); },
    reduced,
  };
})();
