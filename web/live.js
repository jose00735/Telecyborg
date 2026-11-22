// Listening (stage 2) and voice capture for smart follow (stage 3): setup dialog, speech engine
// install, capture, chunk queue, live transcript. Privacy rules: docs/requirements/10-privacy-and-consent.md
(function () {
  "use strict";
  const { $, api, upload, toast, esc } = window.TC;
  const A = window.TCAudio;
  let app = null;
  const L = { sid: "", other: null, me: null, on: false, queue: { other: [], me: [] }, busy: { other: false, me: false }, lines: [], poll: 0, engineOnly: false, starting: false };

  // ---------- speech engine ----------
  async function engineStatus() { return api("GET", "speech/status"); }

  function renderEngine(st) {
    const el = $("lm-engine");
    const sel = (app.settings().speech || {}).model || "base-q5_1";
    const dl = st.download;
    let h = L.engineOnly ? "<p class=\"hint\">whisper.cpp — speech-to-text that runs on this computer. Audio never leaves it.</p>" : "<h3>Speech engine <span class=\"hint\">— whisper.cpp, runs on this computer</span></h3>";
    if (st.external) h += `<p class="ok">Using an external whisper server.</p>`;
    else if (st.ready) {
      const m = st.models.find((x) => x.id === st.model) || {};
      h += `<p class="ok">Ready · ${esc(m.label || st.model)} model${st.runtimeFrom === "downloaded" ? " · runtime downloaded" : ""}.</p>`;
    } else if (!st.runtime && !(dl && dl.active)) {
      h += `<p>Not installed. TeleCyborg downloads the speech runtime (4 MB) and a model once; after that it works offline.</p>`;
    } else if (!(dl && dl.active)) {
      h += `<p>Choose a model to download once. It stays on this computer.</p>`;
    }
    if (st.needsVCRedist) {
      h += `<p class="warn">The Microsoft Visual C++ runtime looks missing; the speech engine needs it. <button class="mini" id="lm-vc">Get it from Microsoft</button></p>`;
    }
    if (!st.external) {
      h += `<div class="models">` + st.models.map((m) =>
        `<label class="model"><input type="radio" name="lm-model" value="${m.id}" ${m.id === (st.model || sel) ? "checked" : ""}>
         <span><b>${esc(m.label)}</b> · ${m.sizeMB} MB · ${esc(m.note)} ${m.installed ? '<em class="ok">installed</em>' : ""}</span></label>`).join("") + `</div>`;
      if (dl && dl.active) {
        const pct = dl.total > 0 ? Math.round((dl.got / dl.total) * 100) : 0;
        h += `<div class="progress"><i style="width:${pct}%"></i></div><p class="hint">Downloading ${esc(dl.what)} — ${(dl.got / 1048576).toFixed(1)} MB${dl.total > 0 ? " of " + (dl.total / 1048576).toFixed(1) + " MB" : ""}</p>`;
      } else {
        h += `<div class="actions left"><button id="lm-install" class="primary">Download selected model</button>
          <label class="btn" for="lm-model-file" tabindex="0">Use a model file I have…</label><input id="lm-model-file" type="file" accept=".bin" hidden>
          ${st.ready ? '<button id="lm-test">Test microphone (4 s)</button>' : ""}</div>`;
      }
      if (dl && dl.error) h += `<p class="warn">Download failed: ${esc(dl.error)}</p>`;
      if (st.lastError) h += `<p class="warn">${esc(st.lastError)}</p>`;
    } else {
      h += `<div class="actions left"><button id="lm-test">Test microphone (4 s)</button></div>`;
    }
    h += `<p class="hint" id="lm-test-out"></p>`;
    el.innerHTML = h;
    const sel2 = () => { const r = el.querySelector('input[name="lm-model"]:checked'); return r ? r.value : "base-q5_1"; };
    el.querySelectorAll('input[name="lm-model"]').forEach((r) => (r.onchange = () => {
      const s = app.settings(); s.speech.model = r.value; app.saveSettings();
      const m = st.models.find((x) => x.id === r.value);
      if (m && m.installed) setTimeout(refreshEngine, 300);
    }));
    const inst = $("lm-install");
    if (inst) inst.onclick = async () => {
      try { renderEngine(await api("POST", "speech/install", { model: sel2() })); watchDownload(); }
      catch (e) { toast(e.message, true); }
    };
    const vc = $("lm-vc");
    if (vc) vc.onclick = () => api("POST", "open", { url: "https://aka.ms/vs/17/release/vc_redist.x64.exe" }).catch((e) => toast(e.message, true));
    const mf = $("lm-model-file");
    if (mf) mf.onchange = async (ev) => {
      const f = ev.target.files[0]; ev.target.value = "";
      if (!f) return;
      $("lm-test-out").textContent = "Copying " + f.name + "…";
      try { const r = await upload("speech/import?name=" + encodeURIComponent(f.name), f); toast("Model added: " + r.file); renderEngine(r.status); }
      catch (e) { $("lm-test-out").textContent = ""; toast(e.message, true); }
    };
    const test = $("lm-test");
    if (test) test.onclick = testMic;
  }
  async function refreshEngine() { try { renderEngine(await engineStatus()); } catch (e) { toast(e.message, true); } }
  function watchDownload() {
    clearInterval(L.poll);
    L.poll = setInterval(async () => {
      let st; try { st = await engineStatus(); } catch (_) { return; }
      if ($("live-modal").hidden) { if (!st.download || !st.download.active) clearInterval(L.poll); return; }
      renderEngine(st);
      if (!st.download || !st.download.active) {
        clearInterval(L.poll);
        if (st.download && st.download.done) toast("Speech engine installed");
      }
    }, 600);
  }

  async function testMic() {
    const out = $("lm-test-out");
    out.textContent = "Listening for 4 seconds — say a sentence…";
    try {
      const pcm = await A.record(micSource(), 4);
      out.textContent = "Transcribing… (the first run loads the model)";
      const lang = $("lm-lang-other").value === "auto" ? "auto" : (app.script() && app.script().parsed.meta.lang) || $("lm-lang-other").value;
      const r = await upload("speech/test?lang=" + encodeURIComponent(lang), pcm.buffer);
      out.innerHTML = r.text ? `Heard: “${esc(r.text)}” <span class="hint">(${r.ms} ms)</span>` : "Heard nothing. Check the microphone in the list above.";
    } catch (e) { out.textContent = e.message; }
  }

  // ---------- setup dialog ----------
  function micSource() {
    const v = $("lm-mic").value;
    return { kind: "mic", deviceId: v && v !== "default" && v !== "off" ? v : "" };
  }

  async function fillDevices() {
    let devs = [];
    try { devs = await A.devices(); } catch (_) { /* ignore */ }
    const sp = app.settings().speech;
    const named = devs.some((d) => d.label && !/^Input \d+$/.test(d.label));
    $("lm-dev-hint").hidden = named;
    const opts = (cur) => `<option value="default">Default microphone</option>` +
      devs.filter((d) => d.id && d.id !== "default").map((d) => `<option value="${esc(d.id)}" ${d.id === cur ? "selected" : ""}>${esc(d.label)}</option>`).join("");
    $("lm-mic").innerHTML = `<option value="off">Off — don't listen to me</option>` + opts(sp.micDevice);
    $("lm-mic").value = sp.micOn ? (devs.some((d) => d.id === sp.micDevice) ? sp.micDevice : "default") : "off";
    $("lm-other-dev").innerHTML = devs.filter((d) => d.id && d.id !== "default").map((d) => `<option value="${esc(d.id)}" ${d.id === sp.otherDevice ? "selected" : ""}>${esc(d.label)}</option>`).join("") || `<option value="">No input devices found</option>`;
  }

  async function openSetup(engineOnly) {
    L.engineOnly = !!engineOnly;
    L.resume = app.pause ? app.pause() : false;
    const sp = app.settings().speech;
    $("lm-title").textContent = engineOnly ? "Speech engine" : "Listening";
    document.querySelectorAll("#live-modal .lm-sec:not(#lm-engine)").forEach((s) => (s.hidden = engineOnly));
    $("lm-start").hidden = engineOnly;
    $("lm-consent").hidden = engineOnly || !!sp.consent;
    $("lm-consent-ok").checked = !!sp.consent;
    $("lm-other").value = sp.otherSource;
    $("lm-other-dev-wrap").hidden = sp.otherSource !== "device";
    $("lm-lang-other").value = sp.langOther;
    $("lm-mode").value = sp.mode;
    $("lm-save").checked = false; // per session (PV-3)
    $("lm-msg").textContent = "";
    $("live-modal").hidden = false;
    $("lm-engine").innerHTML = "<p class=\"hint\">Checking the speech engine…</p>";
    fillDevices();
    try {
      const st = await engineStatus();
      renderEngine(st);
      if (st.download && st.download.active) watchDownload();
    } catch (e) { $("lm-engine").innerHTML = `<p class="warn">${esc(e.message)}</p>`; }
    setTimeout(() => (engineOnly ? $("lm-close") : $("lm-start")).focus(), 30);
  }
  function closeSetup() {
    if ($("live-modal").hidden) return;
    $("live-modal").hidden = true; clearInterval(L.poll);
    if (L.resume && app.resume) app.resume();
    L.resume = false;
  }

  async function start() {
    const msg = $("lm-msg");
    if (!$("lm-consent").hidden && !$("lm-consent-ok").checked) { msg.textContent = "Tick “I understand” first."; return; }
    const sp = app.settings().speech;
    sp.consent = true;
    sp.otherSource = $("lm-other").value;
    sp.otherDevice = $("lm-other-dev").value;
    sp.langOther = $("lm-lang-other").value;
    sp.mode = $("lm-mode").value;
    sp.micOn = $("lm-mic").value !== "off";
    if (sp.micOn) sp.micDevice = $("lm-mic").value === "default" ? "" : $("lm-mic").value;
    app.saveSettings();
    if (sp.otherSource === "off" && !sp.micOn) { msg.textContent = "Turn on at least one side."; return; }
    if (sp.mode !== "clock" && !sp.micOn) { msg.textContent = "Follow and hybrid modes need your microphone."; return; }
    let st;
    try { st = await engineStatus(); } catch (e) { msg.textContent = e.message; return; }
    if (!st.ready) { msg.textContent = "Install the speech engine first (above)."; return; }
    const sc = app.script();
    if (!sc) return;
    msg.textContent = "Starting…";
    L.starting = true;
    try {
      const r = await api("POST", "live/start", { scriptId: sc.id, save: $("lm-save").checked });
      L.sid = r.id;
      L.save = $("lm-save").checked;
      if (sp.otherSource !== "off") {
        L.other = await A.capture(sp.otherSource === "system" ? { kind: "system" } : { kind: "device", deviceId: sp.otherDevice }, {
          maxSec: 7, silenceMs: 650, minSec: 0.6, onChunk: (pcm) => enqueue("other", pcm),
          onEnded: () => { toast("System audio sharing stopped", true); stopSide("other"); },
        });
      }
      if (sp.micOn) {
        L.me = await A.capture(micSource(), {
          maxSec: 2.4, silenceMs: 380, overlapSec: 0.6, minSec: 0.4, onChunk: (pcm) => enqueue("me", pcm),
          onSpeaking: (s) => app.onSpeaking && app.onSpeaking(s),
          onLevel: (v) => { const l = $("pr-lvl"); if (l) l.style.transform = `scaleY(${0.15 + v * 0.85})`; },
        });
      }
    } catch (e) {
      L.starting = false;
      await stop(true);
      msg.textContent = e.name === "NotAllowedError" ? "Access was refused. Allow the microphone / screen audio and try again." : e.message;
      return;
    }
    L.starting = false;
    L.on = true;
    closeSetup();
    app.onListening(true, { mode: sp.mode, me: !!L.me, other: !!L.other, save: L.save });
    toast("Listening" + (L.other ? " to " + L.other.label : "") + (L.me ? (L.other ? " and you" : " to you") : ""));
  }

  function stopSide(role) {
    const src = L[role];
    if (src) { try { src.stop(); } catch (_) { /* ignore */ } }
    L[role] = null;
    L.queue[role] = [];
  }

  // Stop listening; returns the session notes (or null).
  async function stop(silent) {
    const had = L.on || L.sid;
    stopSide("other"); stopSide("me");
    L.on = false;
    const sid = L.sid; L.sid = "";
    let notes = null;
    if (sid) {
      try { notes = await api("POST", "live/" + sid + "/stop", { used: app.usedSections ? app.usedSections() : [] }); } catch (_) { /* ignore */ }
    }
    if (had && app.onListening) app.onListening(false, {});
    if (had && !silent) toast("Stopped listening");
    return notes;
  }

  // ---------- chunks ----------
  function enqueue(role, pcm) {
    if (!L.sid) return;
    const q = L.queue[role];
    q.push(pcm);
    // latency over completeness for my own voice; keep a little more of the other side
    const max = role === "me" ? 2 : 4;
    while (q.length > max) q.shift();
    pump(role);
  }
  async function pump(role) {
    if (L.busy[role] || !L.queue[role].length || !L.sid) return;
    L.busy[role] = true;
    const pcm = L.queue[role].shift();
    const sp = app.settings().speech;
    const sc = app.script();
    const lang = role === "other" ? sp.langOther : (sp.langMe || (sc && sc.parsed.meta.lang) || sp.langOther || "en");
    const track = app.track();
    try {
      const r = await upload(`live/${L.sid}/audio?role=${role}&track=${track}&lang=${encodeURIComponent(lang === "auto" ? "auto" : lang)}`, pcm.buffer);
      handle(r);
    } catch (e) {
      if (e.status === 404) { L.sid = ""; }
      else { app.onEngineError && app.onEngineError(e.message); }
    }
    L.busy[role] = false;
    pump(role);
  }
  function handle(r) {
    if (r.text) caption(r.role, r.text);
    if (r.role === "other" && r.suggestions && r.suggestions.length) app.onSuggestions(r.suggestions);
    if (r.role === "me" && r.position) app.onPosition(r.position, r.track, r);
    if (r.fillers && app.onFillers) app.onFillers(r.fillers);
  }

  async function typed(text) {
    if (!text.trim()) return [];
    if (!L.sid) {
      // no session: start a text-only one so typed questions still get suggestions
      const sc = app.script();
      if (!sc) return [];
      try { L.sid = (await api("POST", "live/start", { scriptId: sc.id, save: false })).id; } catch (e) { toast(e.message, true); return []; }
    }
    try {
      const r = await api("POST", `live/${L.sid}/text`, { role: "other", text });
      caption("other", text);
      return r.suggestions || [];
    } catch (e) { toast(e.message, true); return []; }
  }

  async function seek(track, para) {
    if (!L.sid || !L.me) return;
    try { await api("POST", `live/${L.sid}/seek`, { track, para }); } catch (_) { /* ignore */ }
  }

  function caption(role, text) {
    L.lines.push({ role, text });
    if (L.lines.length > 6) L.lines.shift();
    $("pr-cap-lines").innerHTML = L.lines.map((l) => `<p class="${l.role}"><b>${l.role === "me" ? "You" : "Them"}</b> ${esc(l.text)}</p>`).join("");
  }
  function clearCaptions() { L.lines = []; $("pr-cap-lines").innerHTML = ""; }

  function init(ctx) {
    app = ctx;
    $("lm-close").onclick = closeSetup;
    $("lm-start").onclick = start;
    $("lm-other").onchange = () => ($("lm-other-dev-wrap").hidden = $("lm-other").value !== "device");
    $("lm-unlock").onclick = async () => {
      try { await A.unlockLabels(); await fillDevices(); } catch (e) { toast(e.name === "NotAllowedError" ? "Microphone access was refused" : e.message, true); }
    };
    $("live-modal").addEventListener("keydown", (e) => {
      if (e.key === "Escape") { e.stopPropagation(); closeSetup(); }
    });
  }

  window.TCLive = {
    init, openSetup, closeSetup, start, stop, seek, typed, clearCaptions,
    isOn: () => L.on, hasMe: () => !!L.me, hasSession: () => !!L.sid, isOpen: () => !$("live-modal").hidden,
  };
})();
