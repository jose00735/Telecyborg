// TeleCyborg stage 1 UI. Timing model: docs/requirements/03-pacing-and-timing.md
(function () {
  "use strict";
  const $ = (id) => document.getElementById(id);
  const MIN_WPM = 60, MAX_WPM = 300;

  // ---------- token & API ----------
  let token = "";
  {
    const m = location.hash.match(/t=([0-9a-f]+)/);
    try {
      if (m) { token = m[1]; sessionStorage.setItem("tc-token", token); }
      else token = sessionStorage.getItem("tc-token") || "";
    } catch (_) { if (m) token = m[1]; }
    if (m) history.replaceState(null, "", location.pathname);
  }
  async function api(method, path, body) {
    const res = await fetch("/api/" + path, {
      method, headers: { "X-TC-Token": token, "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (res.status === 204) return null;
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || "Request failed (" + res.status + ")");
    return data;
  }

  // ---------- helpers ----------
  const clampWpm = (n) => Math.max(MIN_WPM, Math.min(MAX_WPM, Math.round(n) || 140));
  function fmt(sec) {
    if (!isFinite(sec)) return "--:--";
    const neg = sec < 0; sec = Math.abs(Math.round(sec));
    return (neg ? "−" : "") + Math.floor(sec / 60) + ":" + String(sec % 60).padStart(2, "0");
  }
  function parseDur(s) {
    s = String(s || "").trim().replace(/s$/i, "");
    if (!s) return 0;
    let t = 0;
    for (const p of s.split(":")) { const v = parseFloat(p); if (!isFinite(v) || v < 0) return NaN; t = t * 60 + v; }
    return t;
  }
  function ago(iso) {
    const d = (Date.now() - new Date(iso).getTime()) / 1000;
    if (d < 60) return "just now";
    if (d < 3600) return Math.floor(d / 60) + " min ago";
    if (d < 86400) return Math.floor(d / 3600) + " h ago";
    return new Date(iso).toLocaleDateString();
  }
  let toastTimer = 0;
  function toast(msg, bad) {
    const t = $("toast");
    t.textContent = msg; t.className = "toast" + (bad ? " bad" : ""); t.hidden = false;
    clearTimeout(toastTimer); toastTimer = setTimeout(() => (t.hidden = true), bad ? 6000 : 2600);
  }
  const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

  // ---------- state ----------
  let settings = { wpm: 140, fontSize: 44, mirror: false, focusLine: 0.38, onTop: false, countdown: 3, motion: "full" };
  let info = { version: "", native: false };
  let current = null; // script being edited / rehearsed
  let screen = "";

  function show(name, opts) {
    const swap = () => {
      document.querySelectorAll(".screen").forEach((s) => (s.hidden = s.id !== "screen-" + name));
      screen = name;
      if (opts && opts.onSwap) opts.onSwap();
      Matrix.setAmbient(name === "library");
      const f = document.querySelector("#screen-" + name + " [autofocus], #screen-" + name + " .primary");
      if (f && !(opts && opts.noFocus)) f.focus({ preventScroll: true });
    };
    return opts && opts.instant ? (swap(), Promise.resolve()) : Matrix.wipe(swap);
  }

  // ---------- library ----------
  async function loadLibrary() {
    let list = [];
    try { list = await api("GET", "scripts"); } catch (e) { toast(e.message, true); }
    const ol = $("script-list");
    ol.innerHTML = "";
    $("empty").hidden = list.length > 0;
    ol.hidden = list.length === 0;
    for (const s of list) {
      const li = document.createElement("li");
      li.className = "row";
      li.innerHTML =
        `<span class="caret">&gt;</span>
         <div class="t"><strong>${esc(s.title)}</strong>
           <span class="meta">${s.words} words · ${s.paragraphs} paragraphs · ${s.wpm} wpm${s.target ? " · target " + fmt(s.target) : ""} · edited ${ago(s.modified)}${s.runs ? " · " + s.runs + " runs" : ""}</span></div>
         <span class="dur">${fmt(s.duration)}</span>
         <span class="ops"><button data-op="go" class="primary">Rehearse</button><button data-op="edit">Edit</button><button data-op="del">Delete</button></span>`;
      li.querySelector('[data-op="go"]').onclick = () => openScript(s.id, "prompter");
      li.querySelector('[data-op="edit"]').onclick = () => openScript(s.id, "editor");
      const del = li.querySelector('[data-op="del"]');
      del.onclick = async () => {
        if (!del.classList.contains("danger")) {
          del.classList.add("danger"); del.textContent = "Confirm delete";
          setTimeout(() => { del.classList.remove("danger"); del.textContent = "Delete"; }, 4000);
          return;
        }
        try { await api("DELETE", "scripts/" + s.id); toast("Deleted “" + s.title + "”"); loadLibrary(); }
        catch (e) { toast(e.message, true); }
      };
      ol.appendChild(li);
    }
  }

  async function openScript(id, where) {
    try { current = await api("GET", "scripts/" + id); }
    catch (e) { toast(e.message, true); return loadLibrary(); }
    if (where === "editor") { fillEditor(); await show("editor"); $("ed-body").focus(); }
    else startPrompter();
  }

  async function importText(name, text) {
    const title = name.replace(/\.(md|markdown|txt)$/i, "").replace(/[_-]+/g, " ").trim();
    try {
      const sc = await api("POST", "scripts", { title: /^---\s*\n[\s\S]*?\btitle:/.test(text) ? "" : title, body: text });
      toast("Imported “" + sc.title + "”");
      current = sc; fillEditor(); show("editor");
    } catch (e) { toast(e.message, true); }
  }

  $("file-import").onchange = async (ev) => {
    const f = ev.target.files[0];
    if (f) importText(f.name, await f.text());
    ev.target.value = "";
  };
  $("btn-import").onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); $("file-import").click(); } };
  let dragDepth = 0;
  document.addEventListener("dragenter", (e) => { if (screen === "library" && e.dataTransfer.types.includes("Files")) { dragDepth++; $("drop").hidden = false; } });
  document.addEventListener("dragleave", () => { if (--dragDepth <= 0) { dragDepth = 0; $("drop").hidden = true; } });
  document.addEventListener("dragover", (e) => e.preventDefault());
  document.addEventListener("drop", async (e) => {
    e.preventDefault(); dragDepth = 0; $("drop").hidden = true;
    if (screen !== "library") return;
    const f = e.dataTransfer.files[0];
    if (!f) return;
    if (!/\.(md|markdown|txt)$/i.test(f.name)) return toast("Drop a .md or .txt file", true);
    importText(f.name, await f.text());
  });

  function newScript() {
    current = { id: "", title: "", wpm: settings.wpm, target: 0, body: "" };
    fillEditor(); show("editor").then(() => $("ed-title").focus());
  }
  $("btn-new").onclick = newScript;
  $("btn-new-empty").onclick = newScript;
  $("btn-sample").onclick = async () => {
    try {
      const sc = await api("POST", "scripts", { title: "Sample — how TeleCyborg works", wpm: 140, target: 0, body: SAMPLE });
      current = sc; startPrompter();
    } catch (e) { toast(e.message, true); }
  };
  $("btn-quit").onclick = async () => {
    try { await api("POST", "quit"); } catch (_) { /* window closes */ }
    document.body.innerHTML = '<p style="padding:40px;color:#00ff41;font-family:Consolas,monospace">TeleCyborg has closed. You can close this tab.</p>';
  };
  function motionLabel() { $("btn-motion").textContent = settings.motion === "reduced" ? "Animations: off" : "Animations: on"; }
  $("btn-motion").onclick = () => {
    settings.motion = settings.motion === "reduced" ? "full" : "reduced";
    Matrix.setReduced(settings.motion === "reduced");
    if (screen === "library") Matrix.setAmbient(true);
    motionLabel(); saveSettings();
  };

  // ---------- editor ----------
  let dirty = false, parseTimer = 0, parsed = null;
  function fillEditor() {
    $("ed-title").value = current.title || "";
    $("ed-body").value = current.body || "";
    setWpm(current.wpm || settings.wpm);
    $("ed-target").value = current.target ? fmt(current.target) : "";
    $("ed-fit-msg").textContent = ""; $("ed-fit-msg").className = "hint";
    setDirty(false); reparse(0);
  }
  function setWpm(v) { v = clampWpm(v); $("ed-wpm").value = v; $("ed-wpm-num").value = v; updateDuration(); }
  function setDirty(v) { dirty = v; $("ed-dirty").hidden = !v; }
  function reparse(delay) {
    clearTimeout(parseTimer);
    parseTimer = setTimeout(async () => {
      try { parsed = await api("POST", "parse", { body: $("ed-body").value }); } catch (e) { return; }
      $("st-words").textContent = parsed.words;
      $("st-paras").textContent = parsed.paragraphs;
      $("st-sections").textContent = parsed.sections;
      $("st-pauses").textContent = (Math.round(parsed.pauses * 10) / 10) + " s";
      updateDuration();
    }, delay);
  }
  function updateDuration() {
    if (!parsed) return;
    const wpm = clampWpm($("ed-wpm").value);
    $("st-duration").textContent = fmt(parsed.words * 60 / wpm + parsed.pauses);
  }
  $("ed-body").addEventListener("input", () => { setDirty(true); reparse(250); });
  $("ed-title").addEventListener("input", () => setDirty(true));
  $("ed-body").addEventListener("paste", (e) => {
    // keep pasted text plain (rich text from Word/Docs arrives as text anyway in a textarea)
    const t = e.clipboardData && e.clipboardData.getData("text/plain");
    if (t == null) return;
    e.preventDefault();
    const el = e.target, a = el.selectionStart, b = el.selectionEnd;
    el.setRangeText(t.replace(/\r\n/g, "\n").replace(/ /g, " "), a, b, "end");
    setDirty(true); reparse(250);
  });
  $("ed-wpm").addEventListener("input", (e) => { setWpm(e.target.value); setDirty(true); });
  $("ed-wpm-num").addEventListener("change", (e) => { setWpm(e.target.value); setDirty(true); });
  $("ed-target").addEventListener("input", () => setDirty(true));
  $("ed-fit").onclick = () => {
    const msg = $("ed-fit-msg");
    const t = parseDur($("ed-target").value);
    if (!t || isNaN(t)) { msg.textContent = "Type a target like 1:30 first."; msg.className = "hint bad"; return; }
    if (!parsed || !parsed.words) { msg.textContent = "Add some text first."; msg.className = "hint bad"; return; }
    const avail = t - parsed.pauses;
    const raw = avail > 0 ? parsed.words * 60 / avail : Infinity;
    setWpm(raw); setDirty(true);
    if (raw > MAX_WPM) { msg.textContent = `Needs ${isFinite(raw) ? Math.round(raw) : "∞"} wpm. Cut text or allow more time.`; msg.className = "hint bad"; }
    else if (raw < MIN_WPM) { msg.textContent = `Needs only ${Math.round(raw)} wpm; set to the minimum of 60.`; msg.className = "hint bad"; }
    else { msg.textContent = `Set to ${clampWpm(raw)} wpm to finish in ${fmt(t)}.`; msg.className = "hint"; }
  };

  async function saveEditor() {
    const t = parseDur($("ed-target").value);
    if (isNaN(t)) { toast("Target time should look like 1:30", true); return false; }
    const body = { title: $("ed-title").value.trim(), body: $("ed-body").value, wpm: clampWpm($("ed-wpm").value), target: t || 0 };
    if (!body.body.trim()) { toast("Write or paste some text before saving", true); return false; }
    try {
      current = current.id ? await api("PUT", "scripts/" + current.id, body) : await api("POST", "scripts", body);
      $("ed-title").value = current.title;
      setDirty(false); toast("Saved");
      return true;
    } catch (e) { toast(e.message, true); return false; }
  }
  $("ed-save").onclick = saveEditor;
  $("ed-rehearse").onclick = async () => { if (await saveEditor()) startPrompter(); };
  $("ed-back").onclick = async () => {
    if (dirty && $("ed-body").value.trim() && !(await saveEditor())) return;
    await loadLibrary(); show("library");
  };

  // ---------- prompter ----------
  const P = {}; // run state
  function buildRun() {
    const blocks = current.parsed.blocks;
    const wpm = current.wpm;
    const paras = [];
    const content = $("pr-content");
    content.innerHTML = "";
    content.style.setProperty("--pr-size", settings.fontSize + "px");
    for (const b of blocks) {
      const el = document.createElement("div");
      if (b.kind === "section") { el.className = "sec"; el.innerHTML = b.html; }
      else if (b.kind === "cue") { el.className = "cue"; el.innerHTML = b.html; }
      else {
        el.className = "para"; el.innerHTML = b.html; el.dataset.i = b.index;
        const read = Math.max(0.6, b.words * 60 / wpm);
        paras.push({ el, words: b.words, read, pause: b.pause || 0, plan: read + (b.pause || 0), text: b.text, actual: 0, skipped: false });
      }
      content.appendChild(el);
    }
    const end = document.createElement("div");
    end.className = "end"; end.textContent = "■ end";
    content.appendChild(end);
    let t = 0;
    for (const p of paras) { p.start = t; t += p.plan; }
    Object.assign(P, {
      paras, total: t, planWpm: wpm, live: wpm, wpmStart: wpm, s: 0, elapsed: 0, playing: false, started: false,
      counting: 0, target: current.target || t, words: paras.reduce((a, p) => a + p.words, 0), done: false,
      lastT: 0, hudT: 0, alert: "", raf: 0, reached: 0,
    });
    // timeline
    const tl = $("pr-timeline"); tl.innerHTML = "";
    for (const p of paras) {
      const seg = document.createElement("div");
      seg.className = "seg"; seg.style.flex = String(p.plan);
      seg.innerHTML = '<span class="fill"></span>';
      p.seg = seg; tl.appendChild(seg);
    }
    $("pr-target").textContent = "/ " + fmt(P.target);
    applyView();
  }

  function measure() {
    for (let i = 0; i < P.paras.length; i++) {
      const p = P.paras[i];
      p.top = p.el.offsetTop;
      const next = P.paras[i + 1];
      p.span = (next ? next.el.offsetTop : p.top + p.el.offsetHeight) - p.top;
    }
    P.viewH = $("pr-view").clientHeight;
  }

  function applyView() {
    $("pr-view").classList.toggle("mirrored", settings.mirror);
    $("pr-mirror").setAttribute("aria-pressed", String(settings.mirror));
    $("pr-ontop").setAttribute("aria-pressed", String(settings.onTop));
    $("pr-ontop").hidden = !info.native;
    document.documentElement.style.setProperty("--focus-y", settings.focusLine * 100 + "%");
    $("pr-content").style.setProperty("--pr-size", settings.fontSize + "px");
    $("pr-focus").style.fontSize = settings.fontSize + "px";
    $("pr-content").style.fontSize = settings.fontSize + "px";
    if (P.paras) { measure(); render(); }
  }

  function paraAt(s) {
    const ps = P.paras;
    let lo = 0, hi = ps.length - 1;
    while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (ps[mid].start <= s) lo = mid; else hi = mid - 1; }
    return lo;
  }

  function render() {
    if (!P.paras.length) return;
    const i = paraAt(P.s), p = P.paras[i];
    const into = P.s - p.start;
    const frac = Math.max(0, Math.min(1, into / p.read));
    let y = p.top + frac * p.span;
    if (P.s >= P.total) y = p.top + p.span;
    const focus = P.viewH * settings.focusLine;
    const ty = Math.round(focus - y);
    $("pr-content").style.transform = `translate3d(-50%, ${ty}px, 0)` + (settings.mirror ? " scaleX(-1)" : "");
    if (P.cur !== i) {
      P.paras.forEach((q, k) => { q.el.classList.toggle("past", k < i); q.el.classList.toggle("now", k === i); });
      P.cur = i;
    }
  }

  function startPrompter() {
    if (P.raf) cancelAnimationFrame(P.raf);
    clearTimeout(P.countTimer);
    // build while the rain covers the screen, so the wipe reveals the script already in place
    show("prompter", { noFocus: true, onSwap: () => { buildRun(); measure(); render(); hud(); } }).then(countdown);
  }

  function countdown() {
    const n = settings.countdown;
    const el = $("pr-count");
    P.counting = 1;
    if (!n) { el.hidden = true; return go(); }
    el.hidden = false;
    let k = n;
    const tick = () => {
      if (!P.counting) { el.hidden = true; return; }
      if (k === 0) { el.hidden = true; P.counting = 0; return go(); }
      Matrix.scramble(el, String(k), 260);
      k--; P.countTimer = setTimeout(tick, 1000);
    };
    tick();
  }
  function go() {
    P.counting = 0; P.started = true; P.playing = true; P.lastT = performance.now();
    $("pr-play").textContent = "Pause";
    cancelAnimationFrame(P.raf); P.raf = requestAnimationFrame(loop);
  }

  function loop(t) {
    if (screen !== "prompter") return;
    const dt = Math.min(0.1, (t - P.lastT) / 1000);
    P.lastT = t;
    if (P.started && !P.done) {
      P.elapsed += dt;
      const i = paraAt(P.s);
      P.paras[i].actual += dt;
      if (i > P.reached) P.reached = i;
      if (P.playing) {
        const p = P.paras[i];
        const inPause = P.s - p.start >= p.read;
        P.s += inPause ? dt : dt * P.live / P.planWpm;
        if (P.s >= P.total) { P.s = P.total; finish(true); }
      }
      render();
      if (t - P.hudT > 100) { hud(); P.hudT = t; }
    }
    P.raf = requestAnimationFrame(loop);
  }

  function remaining() {
    const i = paraAt(P.s), p = P.paras[i];
    const into = P.s - p.start;
    let words = p.words * Math.max(0, 1 - into / p.read);
    let pause = into >= p.read ? Math.max(0, p.plan - into) : p.pause;
    for (let k = i + 1; k < P.paras.length; k++) { words += P.paras[k].words; pause += P.paras[k].pause; }
    return { words, pause };
  }

  function hud() {
    const i = paraAt(P.s), p = P.paras[i];
    $("pr-wpm").textContent = P.live;
    $("pr-time").textContent = fmt(P.elapsed);
    const rest = P.total - P.s;
    const proj = P.elapsed + rest * P.planWpm / P.live;
    $("pr-proj").textContent = fmt(proj);
    $("pr-proj").parentElement.classList.toggle("late", proj > P.target + 1);
    $("pr-para").textContent = (i + 1) + "/" + P.paras.length;
    $("pr-ptime").textContent = fmt(p.actual) + " / " + fmt(p.plan);
    // timeline
    P.paras.forEach((q, k) => {
      const f = k < i ? 1 : k > i ? 0 : Math.min(1, (P.s - q.start) / q.plan);
      q.seg.firstChild.style.width = (q.skipped ? 0 : f * 100) + "%";
      q.seg.classList.toggle("over", q.actual > q.plan * 1.15 + 0.5);
      q.seg.classList.toggle("skip", q.skipped);
    });
    coach();
  }

  function coach() {
    const deficit = P.elapsed - P.s;
    const r = remaining();
    const available = P.target - P.elapsed - r.pause;
    const needed = available > 0 ? r.words * 60 / available : Infinity;
    P.needed = needed;
    let kind = "";
    if (P.alert === "behind" || P.alert === "miss") { if (deficit > 1) kind = needed > MAX_WPM || !isFinite(needed) ? "miss" : "behind"; }
    else if (deficit > 3) kind = needed > MAX_WPM || !isFinite(needed) ? "miss" : "behind";
    if (!kind && deficit < -5 && r.words > 0) kind = "ahead";
    if (kind === "behind" && Math.round(needed) <= P.live) kind = ""; // already fast enough
    const el = $("pr-alert");
    if (kind !== P.alert) { P.alert = kind; el.hidden = !kind; el.className = "alert " + kind; }
    if (kind === "behind") el.innerHTML = `Behind by ${fmt(deficit)}. Speed up to <strong>${Math.round(needed)} wpm</strong> to finish in ${fmt(P.target)}. <span><kbd>A</kbd> apply</span>`;
    else if (kind === "miss") el.innerHTML = `Behind by ${fmt(deficit)}. You can't finish in ${fmt(P.target)} at a readable pace; skip a paragraph with <kbd>→</kbd>.`;
    else if (kind === "ahead") el.innerHTML = `Ahead by ${fmt(-deficit)}. Slow down a little or take a breath.`;
  }

  function applySuggested() {
    if (P.alert !== "behind" || !isFinite(P.needed)) return;
    P.live = clampWpm(Math.ceil(P.needed));
    toast("Pace set to " + P.live + " wpm");
    hud();
  }

  function jump(delta) {
    if (!P.started) return;
    const i = paraAt(P.s);
    let j = Math.max(0, Math.min(P.paras.length - 1, i + delta));
    if (delta < 0 && P.s - P.paras[i].start > 1.5 && delta === -1) j = i; // first press: back to start of paragraph
    for (let k = i + 1; k < j; k++) if (P.paras[k].actual < 0.2) P.paras[k].skipped = true;
    if (j > i && P.paras[i].actual < 0.2) P.paras[i].skipped = true;
    P.paras[j].skipped = false;
    P.s = P.paras[j].start;
    render(); hud();
  }

  function togglePlay() {
    if (P.done) return;
    if (P.counting) { clearTimeout(P.countTimer); P.counting = 0; $("pr-count").hidden = true; return go(); }
    if (!P.started) return go();
    P.playing = !P.playing;
    $("pr-play").textContent = P.playing ? "Pause" : "Play";
  }
  function setLive(d) { P.live = clampWpm(P.live + d); hud(); }
  function fontBy(d) {
    settings.fontSize = Math.max(24, Math.min(110, settings.fontSize + d));
    applyView(); saveSettings();
  }
  async function toggleOnTop() {
    if (!info.native) return toast("Keep on top works in the TeleCyborg window", true);
    try { await api("POST", "window/ontop", { on: !settings.onTop }); settings.onTop = !settings.onTop; applyView(); saveSettings(); }
    catch (e) { toast(e.message, true); }
  }

  async function finish(completed) {
    if (P.done) return;
    P.done = true; P.playing = false;
    clearTimeout(P.countTimer); P.counting = 0; $("pr-count").hidden = true;
    $("pr-alert").hidden = true;
    hud();
    const run = {
      planned: P.total, actual: P.elapsed, target: current.target || 0, wpmStart: P.wpmStart, wpmEnd: P.live, completed,
      paras: P.paras.map((p, k) => ({ planned: p.plan, actual: p.actual, skipped: p.skipped, notReached: !completed && k > P.reached && !p.skipped })),
    };
    if (P.started && P.elapsed > 2) {
      try { await api("POST", "scripts/" + current.id + "/rehearsals", run); } catch (e) { toast(e.message, true); }
    }
    if (!P.started || P.elapsed <= 2) return backFromPrompter();
    setTimeout(() => showReport(run), completed ? 900 : 0);
  }
  async function backFromPrompter() {
    cancelAnimationFrame(P.raf);
    fillEditor(); show("editor");
  }

  $("pr-play").onclick = togglePlay;
  $("pr-faster").onclick = () => setLive(5);
  $("pr-slower").onclick = () => setLive(-5);
  $("pr-big").onclick = () => fontBy(4);
  $("pr-small").onclick = () => fontBy(-4);
  $("pr-mirror").onclick = () => { settings.mirror = !settings.mirror; applyView(); saveSettings(); };
  $("pr-ontop").onclick = toggleOnTop;
  $("pr-exit").onclick = () => finish(false);
  $("pr-view").addEventListener("wheel", (e) => {
    if (!P.started || P.done) return;
    e.preventDefault();
    P.s = Math.max(0, Math.min(P.total, P.s + Math.sign(e.deltaY) * 0.5 * (P.paras[paraAt(P.s)].read > 4 ? 1 : 0.5)));
    render(); hud();
  }, { passive: false });
  $("pr-content").addEventListener("click", (e) => {
    const el = e.target.closest(".para");
    if (!el || !P.started) return;
    const j = +el.dataset.i;
    jump(j - paraAt(P.s));
  });
  window.addEventListener("resize", () => { if (screen === "prompter" && P.paras) { measure(); render(); } });

  document.addEventListener("keydown", (e) => {
    if (screen === "editor" && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); saveEditor(); return; }
    if (screen === "editor" && e.key === "Escape") { $("ed-back").click(); return; }
    if (screen === "report" && e.key === "Escape") { $("rp-library").click(); return; }
    if (screen !== "prompter") return;
    if (e.target.tagName === "INPUT") return;
    const k = e.key;
    const map = {
      " ": togglePlay, ArrowUp: () => setLive(5), ArrowDown: () => setLive(-5),
      ArrowRight: () => jump(1), PageDown: () => jump(1), ArrowLeft: () => jump(-1), PageUp: () => jump(-1),
      Home: () => { if (P.started) { P.s = 0; render(); hud(); } }, Escape: () => finish(false),
      "+": () => fontBy(4), "=": () => fontBy(4), "-": () => fontBy(-4),
      m: () => $("pr-mirror").click(), M: () => $("pr-mirror").click(), t: toggleOnTop, T: toggleOnTop,
      a: applySuggested, A: applySuggested, b: togglePlay, ".": togglePlay,
    };
    if (map[k]) { e.preventDefault(); map[k](); }
  });

  // ---------- report ----------
  async function showReport(run) {
    cancelAnimationFrame(P.raf);
    const diff = run.actual - (run.target || run.planned);
    const goal = run.target || run.planned;
    $("rp-title").textContent = run.completed ? "Run complete" : "Run stopped";
    $("rp-verdict").innerHTML =
      `<span class="num ${diff > 2 ? "late" : ""}">${fmt(run.actual)}</span>` +
      (run.completed
        ? (Math.abs(diff) <= 2 ? `On time against ${fmt(goal)}.` : diff > 0 ? `${fmt(diff)} over ${fmt(goal)}.` : `${fmt(-diff)} under ${fmt(goal)}.`)
        : `Stopped before the end. Planned ${fmt(run.planned)}.`) +
      (run.paras.some((p) => p.skipped) ? ` ${run.paras.filter((p) => p.skipped).length} paragraph(s) skipped.` : "") +
      (run.wpmEnd !== run.wpmStart ? ` Pace changed from ${run.wpmStart} to ${run.wpmEnd} wpm.` : "");
    const rows = $("rp-rows"); rows.innerHTML = "";
    const maxT = Math.max(...run.paras.map((p) => Math.max(p.planned, p.actual)), 1);
    run.paras.forEach((p, i) => {
      const d = p.actual - p.planned;
      const idle = p.skipped || p.notReached;
      const late = !idle && d > p.planned * 0.15 + 0.5;
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${i + 1}</td><td class="txt" title="${esc(P.paras[i].text)}">${esc(P.paras[i].text)}</td>
        <td class="num">${fmt(p.planned)}</td><td class="num ${idle ? "skip" : late ? "late" : ""}">${p.skipped ? "skipped" : p.notReached ? "not reached" : fmt(p.actual)}</td>
        <td><div class="diff ${late ? "late" : ""}"><span class="bar-o"><i style="left:0;width:${(Math.min(p.actual, maxT) / maxT) * 100}%"></i></span><span>${idle ? "" : (d >= 0 ? "+" : "") + Math.round(d) + " s"}</span></div></td>`;
      rows.appendChild(tr);
    });
    const hist = $("rp-history"); hist.innerHTML = "";
    try {
      const runs = await api("GET", "scripts/" + current.id + "/rehearsals");
      runs.slice(1, 8).forEach((r) => {
        const li = document.createElement("li");
        li.textContent = `${new Date(r.at).toLocaleString()} — ${fmt(r.actual)} of ${fmt(r.target || r.planned)}${r.completed ? "" : " (stopped)"} · ${r.wpmStart}→${r.wpmEnd} wpm`;
        hist.appendChild(li);
      });
      if (!hist.children.length) hist.innerHTML = "<li>This was the first run.</li>";
    } catch (_) { /* ignore */ }
    show("report");
  }
  $("rp-again").onclick = () => startPrompter();
  $("rp-edit").onclick = () => { fillEditor(); show("editor"); };
  $("rp-library").onclick = async () => { await loadLibrary(); show("library"); };

  // ---------- settings ----------
  let saveTimer = 0;
  function saveSettings() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => api("PUT", "settings", settings).catch(() => {}), 400);
  }

  // ---------- boot ----------
  async function boot() {
    if (!token) {
      document.body.innerHTML = '<p style="padding:40px;color:#00ff41;font-family:Consolas,monospace">Open TeleCyborg from its window or the address it printed (it carries a one-time key).</p>';
      return;
    }
    try { [settings, info] = await Promise.all([api("GET", "settings"), api("GET", "info")]); } catch (e) { toast(e.message, true); }
    Matrix.setReduced(settings.motion === "reduced");
    motionLabel();
    $("ver").textContent = "TeleCyborg " + info.version;
    $("mode").textContent = info.native ? "" : "Browser mode";
    await loadLibrary();
    if (!settings.seenIntro && !Matrix.reduced()) {
      settings.seenIntro = true; saveSettings();
      await show("library");
    } else {
      show("library", { instant: true });
      Matrix.decrypt(document.querySelector(".brand"), 600);
    }
  }

  const SAMPLE = `# Opening
This is TeleCyborg. Read along the bright line. The text moves at the pace you set, and every paragraph has a time budget.

[pause 1.5]

# Controls
Press **Space** to pause and play. The **up** and **down** arrows change the pace by five words per minute. The **right** and **left** arrows, or a presentation clicker, jump between paragraphs.
> breathe

# Pace coaching
If you fall behind the target time, an amber bar tells you by how much and the pace you need to finish on time. Press **A** to apply it.

[pause 1]

# Make it yours
Press **Escape** to stop. You will see how long each paragraph took against its plan. Then write your own script, set a target time, and press **Fit to time**.`;

  boot();
})();
