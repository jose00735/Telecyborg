// TeleCyborg UI. Timing model: docs/requirements/03-pacing-and-timing.md
// Stage 2 (cues, jump and return) and stage 3 (smart follow) live in the prompter section below.
(function () {
  "use strict";
  const { $, api, fmt, parseDur, ago, toast, esc, fold, copyText } = window.TC;
  const MIN_WPM = 60, MAX_WPM = 300;
  const clampWpm = (n) => Math.max(MIN_WPM, Math.min(MAX_WPM, Math.round(n) || 140));

  // ---------- state ----------
  let settings = { wpm: 140, fontSize: 44, mirror: false, focusLine: 0.38, onTop: false, countdown: 3, motion: "full",
    speech: { model: "base-q5_1", langOther: "en", langMe: "", mode: "clock", otherSource: "system", micOn: true, answerMax: 60, captions: true } };
  let info = { version: "", native: false };
  let current = null; // script being edited / rehearsed
  let screen = "";
  let hideInShare = false;

  function show(name, opts) {
    const swap = () => {
      document.querySelectorAll(".screen").forEach((s) => (s.hidden = s.id !== "screen-" + name));
      screen = name;
      if (opts && opts.onSwap) opts.onSwap();
      Matrix.setAmbient(name === "library" || name === "packs");
      const f = document.querySelector("#screen-" + name + " [autofocus], #screen-" + name + " .primary");
      if (f && !(opts && opts.noFocus)) f.focus({ preventScroll: true });
    };
    return opts && opts.instant ? (swap(), Promise.resolve()) : Matrix.wipe(swap);
  }
  const modalOpen = () => !$("live-modal").hidden || !$("search-modal").hidden;

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
           <span class="meta">${s.words} words · ${s.paragraphs} paragraphs${s.qa ? " · " + s.qa + " Q&amp;A" : ""} · ${s.wpm} wpm${s.target ? " · target " + fmt(s.target) : ""} · edited ${ago(s.modified)}${s.runs ? " · " + s.runs + " runs" : ""}${s.pack ? ' · <span class="tag">pack</span>' : ""}</span></div>
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
    catch (e) { toast(e.message, true); await loadLibrary(); return show("library"); }
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
    try { await window.TCLive.stop(true); } catch (_) { /* ignore */ }
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
  $("btn-engine").onclick = () => window.TCLive.openSetup(true);

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
      $("st-sections").textContent = parsed.sections - parsed.qaItems;
      $("st-pauses").textContent = (Math.round(parsed.pauses * 10) / 10) + " s";
      $("st-qa").textContent = parsed.qaItems;
      $("st-checks").textContent = parsed.checks;
      $("st-checks").parentElement.parentElement.classList.toggle("warn", parsed.checks > 0);
      $("ed-next-check").hidden = !parsed.checks;
      const ol = $("ed-outline");
      ol.innerHTML = parsed.outline.length ? "<h3>Sections &amp; cue keywords</h3>" + parsed.outline.map((o) =>
        `<div class="o ${o.qa ? "qa" : ""}"><b>${esc(o.title)}</b><span>${o.keywords.length ? esc(o.keywords.join(", ")) : "<i>no keywords: matched by title</i>"}</span></div>`).join("") : "";
      updateDuration();
    }, delay);
  }
  function updateDuration() {
    if (!parsed) return;
    const wpm = clampWpm($("ed-wpm").value);
    $("st-duration").textContent = fmt(parsed.words * 60 / wpm + parsed.pauses);
  }
  $("ed-next-check").onclick = () => {
    const ta = $("ed-body"), v = ta.value.toLowerCase();
    let i = v.indexOf("[check]", ta.selectionEnd);
    if (i < 0) i = v.indexOf("[check]");
    if (i < 0) return;
    ta.focus(); ta.setSelectionRange(i, i + 7);
    // scroll the selection into view
    const before = ta.value.slice(0, i).split("\n").length;
    ta.scrollTop = Math.max(0, (before - 4) * parseFloat(getComputedStyle(ta).lineHeight || "27"));
  };
  $("ed-body").addEventListener("input", () => { setDirty(true); reparse(250); });
  $("ed-title").addEventListener("input", () => setDirty(true));
  $("ed-body").addEventListener("paste", (e) => {
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

  // ---------- prompter: tracks ----------
  // A track is a list of paragraphs on one plan clock: the main narrative, or one Q&A answer
  // (a "detour"). Each track keeps its own position s in plan-seconds.
  const P = {}; // run state; P.main is the narrative, P.tr the visible track
  const V = {}; // voice state (smart follow)

  function buildTrack(el, blocks, wpm, sec) {
    el.innerHTML = "";
    el.style.setProperty("--pr-size", settings.fontSize + "px");
    const paras = [];
    for (const b of blocks) {
      const d = document.createElement("div");
      if (b.kind === "section") { d.className = "sec" + (b.qa ? " qa" : ""); d.innerHTML = b.html; d.dataset.sec = b.sec; }
      else if (b.kind === "cue") { d.className = "cue"; d.innerHTML = b.html; }
      else {
        d.className = "para"; d.innerHTML = b.html; d.dataset.i = paras.length;
        const read = Math.max(0.6, b.words * 60 / wpm);
        paras.push({ el: d, words: b.words, read, pause: b.pause || 0, plan: read + (b.pause || 0), text: b.text, actual: 0, skipped: false, sec: b.sec, idx: b.index });
      }
      el.appendChild(d);
    }
    const end = document.createElement("div");
    end.className = "end"; end.textContent = sec < 0 ? "■ end" : "■ end of answer · Backspace returns";
    el.appendChild(end);
    let t = 0;
    for (const p of paras) { p.start = t; t += p.plan; }
    return { el, paras, total: t, s: 0, sec, cur: -1 };
  }

  function buildRun() {
    const blocks = current.parsed.blocks;
    const wpm = current.wpm;
    const main = buildTrack($("pr-content"), blocks.filter((b) => !b.qa), wpm, -1);
    $("pr-answer").hidden = true; $("pr-content").hidden = false; $("pr-answer-head").hidden = true;
    Object.assign(P, {
      main, tr: main, planWpm: wpm, live: wpm, wpmStart: wpm, elapsed: 0, wall: 0, playing: false, started: false,
      counting: 0, target: current.target || main.total, words: main.paras.reduce((a, p) => a + p.words, 0), done: false,
      lastT: 0, hudT: 0, alert: "", raf: 0, reached: 0, stack: [], answers: [], used: new Set(), suggest: [],
      fillers: {}, silences: 0, longest: 0, detour: null, mode: settings.speech.mode, notes: null,
    });
    Object.assign(V, { target: null, track: -1, lastAccept: 0, speaking: false, quietSince: performance.now(), hist: [], wpm: 0, off: false, offTotal: 0, paraWords: {} });
    const tl = $("pr-timeline"); tl.innerHTML = "";
    for (const p of main.paras) {
      const seg = document.createElement("div");
      seg.className = "seg"; seg.style.flex = String(p.plan);
      seg.innerHTML = '<span class="fill"></span>';
      p.seg = seg; tl.appendChild(seg);
    }
    $("pr-target").textContent = "/ " + fmt(P.target);
    renderSuggest(); crumbs(); modeLabel();
    $("pr-search").hidden = false;
    applyView();
  }

  function measure(tr) {
    tr = tr || P.tr;
    for (let i = 0; i < tr.paras.length; i++) {
      const p = tr.paras[i];
      p.top = p.el.offsetTop;
      const next = tr.paras[i + 1];
      p.span = (next ? next.el.offsetTop : p.top + p.el.offsetHeight) - p.top;
    }
    P.viewH = $("pr-view").clientHeight;
  }

  function applyView() {
    $("pr-view").classList.toggle("mirrored", settings.mirror);
    $("pr-mirror").setAttribute("aria-pressed", String(settings.mirror));
    $("pr-ontop").setAttribute("aria-pressed", String(settings.onTop));
    $("pr-ontop").hidden = !info.native;
    $("pr-hide").hidden = !info.native;
    $("pr-hide").setAttribute("aria-pressed", String(hideInShare));
    document.documentElement.style.setProperty("--focus-y", settings.focusLine * 100 + "%");
    for (const id of ["pr-content", "pr-answer"]) { $(id).style.setProperty("--pr-size", settings.fontSize + "px"); $(id).style.fontSize = settings.fontSize + "px"; }
    $("pr-focus").style.fontSize = settings.fontSize + "px";
    $("pr-captions").hidden = !(settings.speech.captions && window.TCLive.isOn());
    if (P.tr) { measure(); render(); }
  }

  function paraAt(tr, s) {
    const ps = tr.paras;
    let lo = 0, hi = ps.length - 1;
    while (lo < hi) { const mid = (lo + hi + 1) >> 1; if (ps[mid].start <= s) lo = mid; else hi = mid - 1; }
    return lo;
  }

  function render() {
    const tr = P.tr;
    if (!tr || !tr.paras.length) return;
    const i = paraAt(tr, tr.s), p = tr.paras[i];
    const into = tr.s - p.start;
    const frac = Math.max(0, Math.min(1, into / p.read));
    let y = p.top + frac * p.span;
    if (tr.s >= tr.total) y = p.top + p.span;
    const focus = P.viewH * settings.focusLine;
    const ty = Math.round(focus - y);
    tr.el.style.transform = `translate3d(-50%, ${ty}px, 0)` + (settings.mirror ? " scaleX(-1)" : "");
    if (tr.cur !== i) {
      tr.paras.forEach((q, k) => { q.el.classList.toggle("past", k < i); q.el.classList.toggle("now", k === i); });
      tr.cur = i;
    }
  }

  function startPrompter() {
    if (P.raf) cancelAnimationFrame(P.raf);
    clearTimeout(P.countTimer);
    show("prompter", { noFocus: true, onSwap: () => { buildRun(); measure(); render(); hud(); } }).then(() => {
      let want = false; try { want = localStorage.getItem("tc-phone-feed") === "1"; } catch (_) { /* ignore */ }
      if (want && !FEED.on) togglePhone();
      countdown();
    });
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

  const voiceDriven = () => P.mode !== "clock" && window.TCLive.hasMe();

  function loop(t) {
    if (screen !== "prompter") return;
    const dt = Math.min(0.1, (t - P.lastT) / 1000);
    P.lastT = t;
    if (P.started && !P.done) {
      const tr = P.tr;
      P.wall += dt;
      if (tr === P.main) P.elapsed += dt;
      else P.detour.time += dt;
      const i = paraAt(tr, tr.s);
      tr.paras[i].actual += dt;
      if (tr === P.main && i > P.reached) P.reached = i;
      if (P.playing) advance(tr, i, dt, t);
      if (tr === P.main && tr.s >= tr.total) { tr.s = tr.total; finish(true); }
      if (tr.s > tr.total) tr.s = tr.total;
      render();
      if (t - P.hudT > 100) { hud(); P.hudT = t; }
    }
    if (FEED.on) feedTick(t, false);
    P.raf = requestAnimationFrame(loop);
  }

  // How the position moves: by the clock, by the voice (follow) or both (hybrid).
  function advance(tr, i, dt, now) {
    const p = tr.paras[i];
    const inPause = tr.s - p.start >= p.read;
    if (!voiceDriven()) { tr.s += inPause ? dt : dt * P.live / P.planWpm; return; }
    const vt = V.track === tr.sec ? V.target : null;
    if (V.speaking && now - V.lastAccept > 8000 && now - V.speakStart > 8000) {
      if (!V.off) { V.off = true; $("pr-offscript").hidden = false; }
    }
    if (V.off) V.offTotal += dt;
    if (P.mode === "follow") {
      if (vt == null) return; // wait for the first words
      let desired = vt + (V.speaking && !V.off ? 0.45 : 0);
      // finished a paragraph: let its [pause] play by the clock
      const vi = paraAt(tr, vt), vp = tr.paras[vi];
      if (vt >= vp.start + vp.read - 0.05 && tr.s >= vp.start + vp.read - 0.3) desired = Math.max(desired, Math.min(vp.start + vp.plan, tr.s + dt));
      if (vt >= tr.total - 0.05 && tr.s > tr.total - 0.4) { tr.s = tr.total; return; }
      if (desired > tr.s) tr.s += (desired - tr.s) * Math.min(1, dt * 3.5);
    } else {
      let rate = inPause ? 1 : P.live / P.planWpm;
      if (vt != null && tr.s > vt + 2.5) rate = 0; // the clock waits for the voice
      tr.s += dt * rate;
      if (vt != null && vt > tr.s + 0.3) tr.s += (vt - tr.s) * Math.min(1, dt * 3);
    }
  }

  function remaining() {
    const M = P.main;
    const i = paraAt(M, M.s), p = M.paras[i];
    const into = M.s - p.start;
    let words = p.words * Math.max(0, 1 - into / p.read);
    let pause = into >= p.read ? Math.max(0, p.plan - into) : p.pause;
    for (let k = i + 1; k < M.paras.length; k++) { words += M.paras[k].words; pause += M.paras[k].pause; }
    return { words, pause };
  }

  function hud() {
    const M = P.main, tr = P.tr;
    const i = paraAt(tr, tr.s), p = tr.paras[i];
    const follow = voiceDriven() && P.mode === "follow";
    $("pr-wpm-k").textContent = follow ? "Spoken" : "Pace";
    $("pr-wpm").textContent = follow ? (V.wpm ? Math.round(V.wpm) : "—") : P.live;
    $("pr-time").textContent = fmt(P.elapsed);
    const rest = M.total - M.s;
    const pace = follow && V.wpm ? V.wpm : P.live;
    const proj = P.elapsed + rest * P.planWpm / pace;
    $("pr-proj").textContent = fmt(proj);
    $("pr-proj").parentElement.classList.toggle("late", proj > P.target + 1);
    $("pr-para").textContent = (tr === M ? "" : "A ") + (i + 1) + "/" + tr.paras.length;
    $("pr-ptime").textContent = fmt(p.actual) + " / " + fmt(p.plan);
    const mi = paraAt(M, M.s);
    M.paras.forEach((q, k) => {
      const f = k < mi ? 1 : k > mi ? 0 : Math.min(1, (M.s - q.start) / q.plan);
      q.seg.firstChild.style.width = (q.skipped ? 0 : f * 100) + "%";
      q.seg.classList.toggle("over", q.actual > q.plan * 1.15 + 0.5);
      q.seg.classList.toggle("skip", q.skipped);
    });
    if (tr === M) coach(); else answerGuard();
  }

  function coach() {
    const M = P.main;
    const deficit = P.elapsed - M.s;
    const r = remaining();
    const available = P.target - P.elapsed - r.pause;
    const needed = available > 0 ? r.words * 60 / available : Infinity;
    P.needed = needed;
    const follow = voiceDriven() && P.mode === "follow";
    const pace = follow && V.wpm ? V.wpm : P.live;
    let kind = "";
    if (P.alert === "behind" || P.alert === "miss") { if (deficit > 1) kind = needed > MAX_WPM || !isFinite(needed) ? "miss" : "behind"; }
    else if (deficit > 3) kind = needed > MAX_WPM || !isFinite(needed) ? "miss" : "behind";
    if (!kind && deficit < -5 && r.words > 0) kind = "ahead";
    if (kind === "behind" && Math.round(needed) <= pace) kind = "";
    const el = $("pr-alert");
    if (kind !== P.alert) { P.alert = kind; el.hidden = !kind; el.className = "alert " + kind; }
    if (kind === "behind") el.innerHTML = follow
      ? `Behind by ${fmt(deficit)}. Speak at about <strong>${Math.round(needed)} wpm</strong> to finish in ${fmt(P.target)}${V.wpm ? " (now " + Math.round(V.wpm) + ")" : ""}.`
      : `Behind by ${fmt(deficit)}. Speed up to <strong>${Math.round(needed)} wpm</strong> to finish in ${fmt(P.target)}. <span><kbd>A</kbd> apply</span>`;
    else if (kind === "miss") el.innerHTML = `Behind by ${fmt(deficit)}. You can't finish in ${fmt(P.target)} at a readable pace; skip a paragraph with <kbd>→</kbd>.`;
    else if (kind === "ahead") el.innerHTML = `Ahead by ${fmt(-deficit)}. Slow down a little or take a breath.`;
  }

  // Answer-length guard for Q&A detours.
  function answerGuard() {
    const D = P.detour, max = settings.speech.answerMax || 60;
    const head = $("pr-answer-head");
    const late = D.time > max, very = D.time > max * 1.5;
    head.className = "answer-head" + (very ? " miss" : late ? " late" : "");
    head.innerHTML = `<span class="q">${esc(D.title)}</span><span class="t">${fmt(D.time)} / ${fmt(D.total)}</span>` +
      (late ? `<span class="w">${very ? "Too long — land it now" : "Wrap up"}</span>` : "") +
      `<span class="k"><kbd>⌫</kbd> back to ${esc(P.stack.length ? P.stack[P.stack.length - 1].label : "narrative")}</span>`;
    const el = $("pr-alert");
    if (P.alert) { P.alert = ""; el.hidden = true; }
  }

  function applySuggested() {
    if (P.alert !== "behind" || !isFinite(P.needed) || (voiceDriven() && P.mode === "follow")) return;
    P.live = clampWpm(Math.ceil(P.needed));
    toast("Pace set to " + P.live + " wpm");
    hud();
  }

  // a manual move: tell the voice follower so it does not pull the script back
  function moved(tr) {
    V.target = tr.s; V.track = tr.sec;
    window.TCLive.seek(tr.sec, paraAt(tr, tr.s));
  }

  function jump(delta) {
    if (!P.started) return;
    const tr = P.tr;
    const i = paraAt(tr, tr.s);
    let j = Math.max(0, Math.min(tr.paras.length - 1, i + delta));
    if (delta === -1 && tr.s - tr.paras[i].start > 1.5) j = i; // first press: back to start of paragraph
    if (tr === P.main) {
      for (let k = i + 1; k < j; k++) if (tr.paras[k].actual < 0.2) tr.paras[k].skipped = true;
      if (j > i && tr.paras[i].actual < 0.2) tr.paras[i].skipped = true;
      tr.paras[j].skipped = false;
    }
    tr.s = tr.paras[j].start;
    moved(tr); render(); hud();
  }

  // ---------- prompter: cue suggestions, jump and return (stage 2) ----------
  const outline = () => (current && current.parsed.outline) || [];

  function onSuggestions(list) {
    if (!P.started || P.done) return;
    const now = performance.now();
    const curMainSec = P.tr === P.main && P.main.paras.length ? P.main.paras[paraAt(P.main, P.main.s)].sec : -2;
    for (const s of list.slice().reverse()) {
      if (P.detour && P.detour.sec === s.sec) continue;
      if (s.sec === curMainSec && P.tr === P.main) continue;
      P.suggest = P.suggest.filter((x) => x.sec !== s.sec);
      P.suggest.unshift(Object.assign({ at: now }, s));
    }
    P.suggest = P.suggest.slice(0, 3);
    renderSuggest();
  }
  function renderSuggest() {
    const ol = $("pr-suggest");
    const now = performance.now();
    P.suggest = (P.suggest || []).filter((x) => now - x.at < 20000);
    ol.innerHTML = P.suggest.map((s, k) =>
      `<li class="${now - s.at > 14000 ? "fading" : ""} ${s.qa ? "qa" : ""}" data-k="${k}"><kbd>${k + 1}</kbd><span class="st">${esc(s.title)}</span><span class="sm">${esc(s.matched.join(", "))}</span></li>`).join("");
    ol.querySelectorAll("li").forEach((li) => (li.onclick = () => acceptSuggestion(+li.dataset.k)));
  }
  setInterval(() => { if (screen === "prompter" && P.suggest && P.suggest.length) renderSuggest(); }, 1000);

  function acceptSuggestion(k) {
    const s = P.suggest && P.suggest[k];
    if (!s) return;
    P.suggest.splice(k, 1); renderSuggest();
    goTo(s.sec);
  }

  function pushReturn() {
    if (P.tr !== P.main) return; // the return point is always in the narrative
    const M = P.main, i = paraAt(M, M.s);
    P.stack.push({ s: M.paras[i].start, label: "¶" + (i + 1) });
  }

  function goTo(sec) {
    const o = outline()[sec];
    if (!o || !P.started) return;
    P.used.add(o.title);
    if (o.qa) return openDetour(sec);
    if (o.firstPara < 0) return;
    pushReturn();
    if (P.detour) closeDetour();
    const M = P.main;
    const j = M.paras.findIndex((p) => p.idx === o.firstPara);
    if (j < 0) return;
    M.s = M.paras[j].start;
    moved(M); render(); hud(); crumbs();
  }

  function openDetour(sec) {
    const o = outline()[sec];
    const blocks = current.parsed.blocks.filter((b) => b.sec === sec);
    if (!blocks.some((b) => b.kind === "para")) return toast("“" + o.title + "” has no answer text", true);
    if (P.detour) recordDetour(); else pushReturn();
    const wpm = P.live;
    const D = buildTrack($("pr-answer"), blocks, wpm, sec);
    P.detour = Object.assign(D, { title: o.title, time: 0 });
    P.tr = D;
    $("pr-content").hidden = true; $("pr-answer").hidden = false; $("pr-answer-head").hidden = false;
    $("pr-alert").hidden = true; P.alert = "";
    V.target = null; V.track = sec;
    window.TCLive.seek(sec, 0);
    measure(D); render(); hud(); crumbs();
    if (!P.playing && P.started && !P.done) togglePlay();
  }
  function recordDetour() {
    const D = P.detour;
    if (D && D.time > 0.5) P.answers.push({ title: D.title, planned: D.total, actual: D.time });
  }
  function closeDetour() {
    recordDetour();
    P.detour = null; P.tr = P.main;
    $("pr-answer").hidden = true; $("pr-content").hidden = false; $("pr-answer-head").hidden = true;
    measure(P.main);
  }

  function back() {
    if (!P.started || P.done) return;
    if (!P.detour && !P.stack.length) return;
    const frame = P.stack.pop();
    if (P.detour) closeDetour();
    if (frame) P.main.s = frame.s;
    P.main.cur = -1;
    moved(P.main); render(); hud(); crumbs();
  }

  function crumbs() {
    const el = $("pr-crumbs");
    if (!P.detour && !(P.stack && P.stack.length)) { el.hidden = true; return; }
    el.hidden = false;
    const here = P.detour ? P.detour.title : (() => { const M = P.main; const p = M.paras[paraAt(M, M.s)]; const o = outline()[p.sec]; return o ? o.title : "¶" + (paraAt(M, M.s) + 1); })();
    el.innerHTML = `<span>Narrative ${esc(P.stack.map((f) => f.label).join(" › "))}</span> › <b>${esc(here)}</b> <kbd>⌫</kbd>`;
  }

  // Ctrl+K: find a section, or type what you were asked to get suggestions.
  let smSel = 0, smItems = [];
  function openSearch() {
    if (screen !== "prompter" || !P.started) return;
    $("search-modal").hidden = false;
    $("sm-input").value = ""; smSel = 0; renderSearch();
    $("sm-input").focus();
  }
  function closeSearch() { $("search-modal").hidden = true; }
  function renderSearch() {
    const q = fold($("sm-input").value.trim());
    const words = q.split(/\s+/).filter(Boolean);
    smItems = outline().filter((o) => (o.paras > 0) && words.every((w) => fold(o.title + " " + o.keywords.join(" ")).includes(w)))
      .map((o) => ({ kind: "sec", sec: o.index, title: o.title, qa: o.qa, sub: o.keywords.join(", ") }));
    if (q.length > 3) smItems.push({ kind: "ask", title: "Suggest from what they asked: “" + $("sm-input").value.trim() + "”" });
    smSel = Math.min(smSel, Math.max(0, smItems.length - 1));
    $("sm-list").innerHTML = smItems.map((it, k) =>
      `<li class="${k === smSel ? "sel" : ""} ${it.qa ? "qa" : ""} ${it.kind}" data-k="${k}"><b>${esc(it.title)}</b>${it.sub ? `<span>${esc(it.sub)}</span>` : ""}</li>`).join("") ||
      `<li class="none">No section matches.</li>`;
    $("sm-list").querySelectorAll("li[data-k]").forEach((li) => (li.onclick = () => { smSel = +li.dataset.k; pickSearch(); }));
  }
  async function pickSearch() {
    const it = smItems[smSel];
    if (!it) return;
    if (it.kind === "ask") {
      const list = await window.TCLive.typed($("sm-input").value.trim());
      closeSearch();
      if (list.length) onSuggestions(list); else toast("No section matched those words", true);
      return;
    }
    closeSearch();
    goTo(it.sec);
  }
  $("sm-input").addEventListener("input", () => { smSel = 0; renderSearch(); });
  $("sm-input").addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") { e.preventDefault(); smSel = Math.min(smItems.length - 1, smSel + 1); renderSearch(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); smSel = Math.max(0, smSel - 1); renderSearch(); }
    else if (e.key === "Enter") { e.preventDefault(); pickSearch(); }
    else if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); closeSearch(); }
  });
  $("search-modal").addEventListener("click", (e) => { if (e.target.id === "search-modal") closeSearch(); });
  $("live-modal").addEventListener("click", (e) => { if (e.target.id === "live-modal") window.TCLive.closeSetup(); });

  // ---------- prompter: voice (stage 3) ----------
  function onPosition(pos, track) {
    if (!P.started || P.done) return;
    const tr = track === -1 ? P.main : (P.detour && P.detour.sec === track ? P.detour : null);
    if (!tr || !pos.accepted) return;
    const now = performance.now();
    V.lastAccept = now;
    if (V.off) { V.off = false; $("pr-offscript").hidden = true; }
    const p = tr.paras[pos.para];
    if (!p) return;
    const frac = pos.paraWords ? Math.min(1, pos.inPara / pos.paraWords) : 1;
    const t = p.start + frac * p.read;
    if (V.track !== track || V.target == null || t > V.target) { V.target = t; V.track = track; }
    // spoken pace over the last 20 s (main narrative only)
    if (track === -1 && pos.moved) {
      V.hist.push({ t: now, w: pos.word });
      while (V.hist.length > 2 && now - V.hist[0].t > 20000) V.hist.shift();
      const a = V.hist[0], b = V.hist[V.hist.length - 1];
      if (b.t - a.t > 4000 && b.w > a.w) V.wpm = (b.w - a.w) / ((b.t - a.t) / 60000);
    }
  }
  function onSpeaking(on) {
    const now = performance.now();
    if (on) {
      const gap = (now - V.quietSince) / 1000;
      if (P.started && !P.done && P.playing && gap > 3 && V.quietSince) { P.silences++; P.longest = Math.max(P.longest, gap); }
      V.speakStart = now;
    } else V.quietSince = now;
    V.speaking = on;
  }
  function onFillers(f) { for (const k in f) P.fillers[k] = (P.fillers[k] || 0) + f[k]; }

  function onListening(on, o) {
    $("pr-listening").hidden = !on;
    $("pr-listen").setAttribute("aria-pressed", String(on));
    $("pr-listen").classList.toggle("on", on);
    $("pr-captions").hidden = !(on && settings.speech.captions);
    if (!on) { $("pr-offscript").hidden = true; V.off = false; }
    if (on) { P.mode = o.mode; modeLabel(); V.quietSince = performance.now(); V.lastAccept = performance.now(); }
    else window.TCLive.clearCaptions();
    if (P.main) { measure(); render(); }
  }

  function modeLabel() {
    const m = P.mode || settings.speech.mode;
    $("pr-mode").textContent = { clock: "Clock", follow: "Follow voice", hybrid: "Hybrid" }[m];
    $("pr-mode").title = m !== "clock" && !window.TCLive.hasMe() ? "Turn on listening to yourself (L) for this mode" : "";
    $("pr-mode").classList.toggle("dim", m !== "clock" && !window.TCLive.hasMe());
  }
  function cycleMode() {
    const order = ["clock", "follow", "hybrid"];
    P.mode = order[(order.indexOf(P.mode) + 1) % 3];
    settings.speech.mode = P.mode; saveSettings();
    if (P.mode !== "clock" && !window.TCLive.hasMe()) toast("Follow modes need listening to your microphone: press L", true);
    else toast({ clock: "Scrolling by the clock", follow: "Following your voice", hybrid: "Clock that waits for your voice" }[P.mode]);
    if (P.tr) { V.target = P.tr.s; V.track = P.tr.sec; }
    modeLabel();
  }

  // ---------- camera self-view (V): read over your own webcam image ----------
  // Prefers "Macawlink Camera" (the phone as webcam). Only you see this window; the call gets
  // the camera's clean picture.
  const CAM = { stream: null, saveFocus: 0 };
  async function toggleCam() {
    if (CAM.stream) return stopCam();
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) return toast("No camera access in this window", true);
    try {
      let devs = (await navigator.mediaDevices.enumerateDevices()).filter((d) => d.kind === "videoinput");
      if (devs.length && !devs[0].label) { const t = await navigator.mediaDevices.getUserMedia({ video: true }); t.getTracks().forEach((x) => x.stop()); devs = (await navigator.mediaDevices.enumerateDevices()).filter((d) => d.kind === "videoinput"); }
      const pick = devs.find((d) => /macawlink/i.test(d.label)) || devs[0];
      if (!pick) return toast("No camera found", true);
      CAM.stream = await navigator.mediaDevices.getUserMedia({ video: { deviceId: { exact: pick.deviceId }, width: { ideal: 1280 }, height: { ideal: 720 } } });
      const v = $("pr-video"); v.srcObject = CAM.stream; v.hidden = false;
      $("pr-view").classList.add("camview");
      $("pr-cam").setAttribute("aria-pressed", "true");
      CAM.saveFocus = settings.focusLine; settings.focusLine = 0.2; // read right under the lens
      applyView();
      toast("Camera: " + (pick.label || "camera") + " — only you see the text");
    } catch (e) { stopCam(); toast(e.name === "NotAllowedError" ? "Camera access was refused" : e.message, true); }
  }
  function stopCam() {
    if (CAM.stream) CAM.stream.getTracks().forEach((t) => t.stop());
    if (CAM.saveFocus) settings.focusLine = CAM.saveFocus;
    CAM.stream = null; CAM.saveFocus = 0;
    const v = $("pr-video"); v.srcObject = null; v.hidden = true;
    $("pr-view").classList.remove("camview");
    $("pr-cam").setAttribute("aria-pressed", "false");
    applyView();
  }

  // ---------- Macawlink phone feed (P): the prompter over the phone's camera preview ----------
  const FEED = { on: false, scriptKey: "", t: 0, last: null, keyT: 0, busyKeys: false };
  async function togglePhone() {
    if (FEED.on) return stopPhone(true);
    let st;
    try { st = await api("GET", "macaw/status"); } catch (e) { return toast(e.message, true); }
    if (!st.prompter) return toast(st.error || "Macawlink is not running on this PC", true);
    FEED.on = true; FEED.scriptKey = "";
    $("pr-phone").classList.add("on"); $("pr-phone").setAttribute("aria-pressed", "true");
    try { localStorage.setItem("tc-phone-feed", "1"); } catch (_) { /* ignore */ }
    toast(st.phone ? "Prompter on " + (st.device || "the phone") : "Prompter will appear on the phone when it streams");
    feedTick(performance.now(), true);
  }
  function stopPhone(user) {
    if (!FEED.on) return;
    FEED.on = false;
    $("pr-phone").classList.remove("on"); $("pr-phone").setAttribute("aria-pressed", "false");
    api("POST", "macaw/clear").catch(() => {});
    if (user) { try { localStorage.removeItem("tc-phone-feed"); } catch (_) { /* ignore */ } }
  }
  function feedScript(tr) {
    const blocks = current.parsed.blocks.filter((b) => (tr.sec < 0 ? !b.qa : b.sec === tr.sec))
      .map((b) => ({ k: b.kind === "section" ? "sec" : b.kind, t: b.text }));
    return { id: current.id + (tr.sec < 0 ? "" : "#" + tr.sec), title: tr.sec < 0 ? current.title : P.detour.title,
      track: tr.sec < 0 ? "main" : "answer", blocks, font_scale: settings.fontSize / 44, mirror: false };
  }
  function feedTick(now, force) {
    if (!FEED.on || !P.tr) return;
    const tr = P.tr;
    const key = current.id + "|" + tr.sec + "|" + current.modified;
    if (key !== FEED.scriptKey) {
      FEED.scriptKey = key; FEED.last = null;
      api("POST", "macaw/script", feedScript(tr)).catch((e) => { toast(e.message, true); stopPhone(false); });
    }
    if (!force && now - FEED.t < 125) return;
    const dt = (now - FEED.t) / 1000;
    FEED.t = now;
    const i = paraAt(tr, tr.s), p = tr.paras[i];
    const frac = Math.max(0, Math.min(1, (tr.s - p.start) / p.read));
    let speed = 0;
    if (FEED.last && FEED.last.i === i && dt > 0 && dt < 1) speed = Math.max(0, (frac - FEED.last.frac) / dt);
    FEED.last = { i, frac };
    const alert = tr === P.main ? P.alert : (P.detour && P.detour.time > (settings.speech.answerMax || 60) ? "wrap" : "");
    api("POST", "macaw/state", {
      id: current.id + (tr.sec < 0 ? "" : "#" + tr.sec), para: i, frac, speed: P.playing ? speed : 0, playing: P.playing && !P.done,
      wpm: voiceDriven() && P.mode === "follow" && V.wpm ? Math.round(V.wpm) : P.live,
      clock: tr === P.main ? fmt(P.elapsed) + " / " + fmt(P.target) : fmt(P.detour.time) + " / " + fmt(tr.total), alert,
      hint: alert === "behind" && isFinite(P.needed) ? "speed up to " + Math.round(P.needed) + " wpm" : "",
    }).catch(() => {});
    if (now - FEED.keyT > 300 && !FEED.busyKeys) {
      FEED.keyT = now; FEED.busyKeys = true;
      api("GET", "macaw/keys").then((r) => {
        for (const k of r.keys || []) {
          ({ toggle: togglePlay, next: () => jump(1), prev: () => jump(-1), faster: () => setLive(5), slower: () => setLive(-5), back })[k]?.();
        }
      }).catch(() => {}).finally(() => (FEED.busyKeys = false));
    }
  }

  function toggleListen() {
    if (window.TCLive.isOn()) window.TCLive.stop(false);
    else window.TCLive.openSetup(false);
  }

  function togglePlay() {
    if (P.done) return;
    if (P.counting) { clearTimeout(P.countTimer); P.counting = 0; $("pr-count").hidden = true; return go(); }
    if (!P.started) return go();
    P.playing = !P.playing;
    $("pr-play").textContent = P.playing ? "Pause" : "Play";
  }
  // pause for a dialog; returns true when the run was moving (so closing the dialog resumes it)
  function pause() {
    if (screen !== "prompter" || P.done) return false;
    let was = P.playing;
    if (P.counting) { clearTimeout(P.countTimer); P.counting = 0; $("pr-count").hidden = true; was = true; P.started = true; P.lastT = performance.now(); cancelAnimationFrame(P.raf); P.raf = requestAnimationFrame(loop); }
    if (P.playing) { P.playing = false; $("pr-play").textContent = "Play"; }
    return was;
  }
  function resume() {
    if (screen !== "prompter" || P.done || P.playing) return;
    P.playing = true; $("pr-play").textContent = "Pause";
  }
  function setLive(d) { P.live = clampWpm(P.live + d); hud(); }
  function fontBy(d) {
    settings.fontSize = Math.max(24, Math.min(110, settings.fontSize + d));
    applyView(); saveSettings();
    if (P.detour) measure(P.detour);
  }
  async function toggleOnTop() {
    if (!info.native) return toast("Keep on top works in the TeleCyborg window", true);
    try { await api("POST", "window/ontop", { on: !settings.onTop }); settings.onTop = !settings.onTop; applyView(); saveSettings(); }
    catch (e) { toast(e.message, true); }
  }
  async function toggleHide() {
    if (!info.native) return toast("Hiding from screen share works in the TeleCyborg window", true);
    try {
      await api("POST", "window/capture", { hide: !hideInShare }); hideInShare = !hideInShare; applyView();
      toast(hideInShare ? "Hidden from screen sharing and recordings" : "Visible in screen sharing");
    } catch (e) { toast(e.message, true); }
  }

  async function finish(completed) {
    if (P.done) return;
    P.done = true; P.playing = false;
    clearTimeout(P.countTimer); P.counting = 0; $("pr-count").hidden = true;
    $("pr-alert").hidden = true;
    if (P.detour) { recordDetour(); }
    hud();
    if (CAM.stream) stopCam();
    if (FEED.on) { stopPhone(false); }
    const followed = voiceDriven();
    const notes = window.TCLive.hasSession() ? await window.TCLive.stop(true) : null;
    const M = P.main;
    const run = {
      planned: M.total, actual: P.elapsed, target: current.target || 0, wpmStart: P.wpmStart, wpmEnd: P.live, completed,
      paras: M.paras.map((p, k) => ({
        planned: p.plan, actual: p.actual, skipped: p.skipped, notReached: !completed && k > P.reached && !p.skipped,
        wpm: followed && p.actual > 1 && !p.skipped ? Math.round(p.words * 60 / Math.max(0.5, p.actual - p.pause)) : 0,
      })),
      mode: followed ? P.mode : "clock", wall: P.wall, answers: P.answers, fillers: notes ? notes.fillers : P.fillers,
      silences: P.silences, longest: Math.round(P.longest * 10) / 10, offScript: Math.round(V.offTotal), spokenWpm: followed && V.wpm ? Math.round(V.wpm) : 0,
    };
    P.notes = notes;
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
  $("pr-hide").onclick = toggleHide;
  $("pr-exit").onclick = () => finish(false);
  $("pr-listen").onclick = toggleListen;
  $("pr-mode").onclick = cycleMode;
  $("pr-search").onclick = openSearch;
  $("pr-cam").onclick = toggleCam;
  $("pr-phone").onclick = togglePhone;
  $("pr-view").addEventListener("wheel", (e) => {
    if (!P.started || P.done) return;
    e.preventDefault();
    const tr = P.tr;
    tr.s = Math.max(0, Math.min(tr.total, tr.s + Math.sign(e.deltaY) * 0.5 * (tr.paras[paraAt(tr, tr.s)].read > 4 ? 1 : 0.5)));
    moved(tr); render(); hud();
  }, { passive: false });
  for (const id of ["pr-content", "pr-answer"]) {
    $(id).addEventListener("click", (e) => {
      const el = e.target.closest(".para");
      if (!el || !P.started) return;
      jump(+el.dataset.i - paraAt(P.tr, P.tr.s));
    });
  }
  window.addEventListener("resize", () => { if (screen === "prompter" && P.tr) { measure(); render(); } });

  document.addEventListener("keydown", (e) => {
    if (!$("search-modal").hidden || !$("live-modal").hidden || !$("sync-modal").hidden) return;
    if (screen === "editor" && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); saveEditor(); return; }
    if (screen === "editor" && e.key === "Escape") { $("ed-back").click(); return; }
    if (screen === "pack" && (e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "s") { e.preventDefault(); $("pf-save").click(); return; }
    if (screen === "report" && e.key === "Escape") { $("rp-library").click(); return; }
    if (screen !== "prompter") return;
    if (e.target.tagName === "INPUT" || e.target.tagName === "TEXTAREA" || e.target.tagName === "SELECT") return;
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") { e.preventDefault(); return openSearch(); }
    if (e.ctrlKey || e.metaKey || e.altKey) return;
    const k = e.key;
    const map = {
      " ": togglePlay, ArrowUp: () => setLive(5), ArrowDown: () => setLive(-5),
      ArrowRight: () => jump(1), PageDown: () => jump(1), ArrowLeft: () => jump(-1), PageUp: () => jump(-1),
      Home: () => { if (P.started) { P.tr.s = 0; moved(P.tr); render(); hud(); } }, Escape: () => finish(false),
      "+": () => fontBy(4), "=": () => fontBy(4), "-": () => fontBy(-4),
      m: () => $("pr-mirror").click(), M: () => $("pr-mirror").click(), t: toggleOnTop, T: toggleOnTop,
      a: applySuggested, A: applySuggested, b: togglePlay, ".": togglePlay,
      l: toggleListen, L: toggleListen, f: cycleMode, F: cycleMode, "/": openSearch,
      c: () => { settings.speech.captions = !settings.speech.captions; saveSettings(); applyView(); },
      C: () => { settings.speech.captions = !settings.speech.captions; saveSettings(); applyView(); },
      h: toggleHide, H: toggleHide, Backspace: back,
      v: toggleCam, V: toggleCam, p: togglePhone, P: togglePhone,
      1: () => acceptSuggestion(0), 2: () => acceptSuggestion(1), 3: () => acceptSuggestion(2),
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
      (run.wpmEnd !== run.wpmStart && run.mode === "clock" ? ` Pace changed from ${run.wpmStart} to ${run.wpmEnd} wpm.` : "") +
      (run.answers.length ? ` Plus ${fmt(run.wall - run.actual)} answering questions.` : "");
    const spoken = run.paras.some((p) => p.wpm);
    $("rp-head-spoken").hidden = !spoken;
    const rows = $("rp-rows"); rows.innerHTML = "";
    const maxT = Math.max(...run.paras.map((p) => Math.max(p.planned, p.actual)), 1);
    run.paras.forEach((p, i) => {
      const d = p.actual - p.planned;
      const idle = p.skipped || p.notReached;
      const late = !idle && d > p.planned * 0.15 + 0.5;
      const tr = document.createElement("tr");
      tr.innerHTML = `<td>${i + 1}</td><td class="txt" title="${esc(P.main.paras[i].text)}">${esc(P.main.paras[i].text)}</td>
        <td class="num">${fmt(p.planned)}</td><td class="num ${idle ? "skip" : late ? "late" : ""}">${p.skipped ? "skipped" : p.notReached ? "not reached" : fmt(p.actual)}</td>
        ${spoken ? `<td class="num">${p.wpm || ""}</td>` : ""}
        <td><div class="diff ${late ? "late" : ""}"><span class="bar-o"><i style="left:0;width:${(Math.min(p.actual, maxT) / maxT) * 100}%"></i></span><span>${idle ? "" : (d >= 0 ? "+" : "") + Math.round(d) + " s"}</span></div></td>`;
      rows.appendChild(tr);
    });
    // stage 2 and 3 details
    const ex = [];
    if (run.mode !== "clock") {
      ex.push(`<p><b>${run.mode === "follow" ? "Smart follow" : "Hybrid"}</b>${run.spokenWpm ? ` · spoken pace ${run.spokenWpm} wpm` : ""}${run.offScript ? ` · off script ${fmt(run.offScript)}` : ""}` +
        `${run.silences ? ` · ${run.silences} silence(s) over 3 s, longest ${Math.round(run.longest)} s` : " · no long silences"}</p>`);
    }
    const fl = Object.entries(run.fillers || {}).sort((a, b) => b[1] - a[1]);
    if (fl.length) ex.push(`<p><b>Filler words</b> · ${fl.map(([w, n]) => `${esc(w)} ×${n}`).join(", ")}</p>`);
    if (run.answers.length) {
      const max = settings.speech.answerMax || 60;
      ex.push(`<h2>Answers</h2><table class="rp-table"><thead><tr><th>Question</th><th class="num">Planned</th><th class="num">Actual</th></tr></thead><tbody>` +
        run.answers.map((a) => `<tr><td class="txt">${esc(a.title)}</td><td class="num">${fmt(a.planned)}</td><td class="num ${a.actual > max ? "late" : ""}">${fmt(a.actual)}</td></tr>`).join("") + `</tbody></table>`);
    }
    $("rp-extra").innerHTML = ex.join("");
    const nb = $("rp-notes-box");
    nb.hidden = !P.notes;
    if (P.notes) {
      $("rp-notes").textContent = P.notes.notes;
      $("rp-notes-saved").textContent = P.notes.saved ? "Saved to " + P.notes.saved : "Not saved (turn on “Save transcript” to keep notes)";
    }
    const hist = $("rp-history"); hist.innerHTML = "";
    try {
      const runs = await api("GET", "scripts/" + current.id + "/rehearsals");
      runs.slice(1, 8).forEach((r) => {
        const li = document.createElement("li");
        li.textContent = `${new Date(r.at).toLocaleString()} — ${fmt(r.actual)} of ${fmt(r.target || r.planned)}${r.completed ? "" : " (stopped)"} · ${r.mode && r.mode !== "clock" ? r.mode + (r.spokenWpm ? " " + r.spokenWpm + " wpm" : "") : r.wpmStart + "→" + r.wpmEnd + " wpm"}${r.answers && r.answers.length ? " · " + r.answers.length + " answers" : ""}`;
        hist.appendChild(li);
      });
      if (!hist.children.length) hist.innerHTML = "<li>This was the first run.</li>";
    } catch (_) { /* ignore */ }
    show("report");
  }
  $("rp-again").onclick = () => startPrompter();
  $("rp-edit").onclick = () => { fillEditor(); show("editor"); };
  $("rp-library").onclick = async () => { await loadLibrary(); show("library"); };
  $("rp-notes-copy").onclick = async () => toast((await copyText($("rp-notes").textContent)) ? "Notes copied" : "Select the notes and copy them");

  // ---------- settings ----------
  let saveTimer = 0;
  function saveSettings() {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => api("PUT", "settings", settings).catch(() => {}), 400);
  }

  // ---------- boot ----------
  async function boot() {
    if (!window.TC.token) {
      document.body.innerHTML = '<p style="padding:40px;color:#00ff41;font-family:Consolas,monospace">Open TeleCyborg from its window or the address it printed (it carries a one-time key).</p>';
      return;
    }
    try { [settings, info] = await Promise.all([api("GET", "settings"), api("GET", "info")]); } catch (e) { toast(e.message, true); }
    Matrix.setReduced(settings.motion === "reduced");
    motionLabel();
    $("ver").textContent = "TeleCyborg " + info.version;
    $("mode").textContent = info.native ? "" : "Browser mode";
    window.TCPacks.init({ show, loadLibrary, openScript });
    window.TCSync.init();
    window.TCLive.init({
      settings: () => settings, saveSettings, script: () => current, track: () => (P.tr ? P.tr.sec : -1),
      onSuggestions, onPosition, onSpeaking, onFillers, onListening, pause, resume,
      usedSections: () => (P.used ? Array.from(P.used) : []),
      onEngineError: (m) => { if (!P.engineErrAt || performance.now() - P.engineErrAt > 15000) { P.engineErrAt = performance.now(); toast(m, true); } },
    });
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

# Listening
keywords: listen, microphone, suggestions
Press **L** to listen. When the other person asks something your script answers, a suggestion appears at the top. Press **1** to jump to it and **Backspace** to come back here. Press **F** to make the text follow your voice.

# Make it yours
Press **Escape** to stop. You will see how long each paragraph took against its plan. Then write your own script, or build one with Claude from a content pack.

=== QA ===
# Q: How does TeleCyborg know what to suggest?
keywords: how does it work, suggest, know
Each section has keywords. When the other side says them, that section is suggested. Answers like this one stay out of the main flow until you need them.`;

  boot();
})();
