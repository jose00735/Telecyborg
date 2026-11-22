// Content packs (stage 2a): pack form → brief for Claude → import Claude's answer as a script.
(function () {
  "use strict";
  const { $, api, upload, fmt, toast, esc, ago, copyText } = window.TC;
  let app = null; // callbacks from app.js
  let cur = null; // pack being edited
  let dirty = false;
  let lastPreview = null;

  const fields = ["occasion", "lang", "target", "wpm", "qa", "ansmax", "audience", "goal", "topics", "facts", "avoid", "sources"];

  async function list() {
    let packs = [];
    try { packs = await api("GET", "packs"); } catch (e) { toast(e.message, true); }
    const ol = $("pk-list"); ol.innerHTML = "";
    $("pk-empty").hidden = packs.length > 0; ol.hidden = !packs.length;
    for (const p of packs) {
      const li = document.createElement("li");
      li.className = "row";
      li.innerHTML = `<span class="caret">&gt;</span>
        <div class="t"><strong>${esc(p.title)}</strong>
        <span class="meta">${esc(p.occasion)} · ${esc(p.target)} · ${p.topics.length} topics · ${p.qaCount} questions · ${p.scriptId ? "script ready" : "waiting for Claude's answer"} · edited ${ago(p.modified)}</span></div>
        <span class="dur">${esc(p.target)}</span>
        <span class="ops">${p.scriptId ? '<button data-op="go" class="primary">Rehearse</button>' : ""}<button data-op="open">Open</button><button data-op="exp">Export</button><button data-op="del">Delete</button></span>`;
      li.querySelector('[data-op="open"]').onclick = () => openPack(p.id);
      const go = li.querySelector('[data-op="go"]');
      if (go) go.onclick = () => app.openScript(p.scriptId, "prompter");
      li.querySelector('[data-op="exp"]').onclick = () => exportPack(p.id);
      const del = li.querySelector('[data-op="del"]');
      del.onclick = async () => {
        if (!del.classList.contains("danger")) {
          del.classList.add("danger"); del.textContent = "Confirm delete";
          setTimeout(() => { del.classList.remove("danger"); del.textContent = "Delete"; }, 4000);
          return;
        }
        try { await api("DELETE", "packs/" + p.id); toast("Pack deleted; its script stays in the library"); list(); }
        catch (e) { toast(e.message, true); }
      };
      ol.appendChild(li);
    }
  }

  async function exportPack(id) {
    try { const r = await api("POST", "packs/" + id + "/export"); toast("Saved " + r.path); }
    catch (e) { toast(e.message, true); }
  }

  function fill(p) {
    cur = p;
    $("pf-title").value = p.title || "";
    $("pf-occasion").value = p.occasion || "interview";
    $("pf-lang").value = p.lang || "en";
    $("pf-target").value = p.target || "2:00";
    $("pf-wpm").value = p.wpm || 140;
    $("pf-qa").value = p.qaCount || 8;
    $("pf-ansmax").value = p.answerMax || 60;
    $("pf-audience").value = p.audience || "";
    $("pf-goal").value = p.goal || "";
    $("pf-topics").value = (p.topics || []).join("\n");
    $("pf-facts").value = (p.facts || []).join("\n");
    $("pf-avoid").value = (p.avoid || []).join("\n");
    $("pf-sources").value = p.sources || "";
    $("pf-brief-text").value = "";
    $("pf-copy").disabled = true;
    $("pf-answer").value = "";
    $("pf-preview").hidden = true;
    $("pf-import").disabled = true;
    $("pf-import").textContent = p.scriptId ? "Update script" : "Create script";
    $("pf-done").hidden = !p.scriptId;
    $("pf-done-msg").textContent = p.scriptId ? "This pack has a script. Paste a new answer above to replace it." : "";
    lastPreview = null;
    setDirty(false);
  }
  function read() {
    const lines = (id) => $(id).value.split("\n").map((s) => s.trim()).filter(Boolean);
    return Object.assign({}, cur, {
      title: $("pf-title").value.trim(), occasion: $("pf-occasion").value, lang: $("pf-lang").value,
      target: $("pf-target").value.trim(), wpm: +$("pf-wpm").value || 140, qaCount: +$("pf-qa").value || 8,
      answerMax: +$("pf-ansmax").value || 60, audience: $("pf-audience").value.trim(), goal: $("pf-goal").value.trim(),
      topics: lines("pf-topics"), facts: lines("pf-facts"), avoid: lines("pf-avoid"), sources: $("pf-sources").value,
    });
  }
  function setDirty(v) { dirty = v; $("pf-dirty").hidden = !v; }

  async function save() {
    const p = read();
    if (!p.title) { toast("Give the pack a title", true); $("pf-title").focus(); return null; }
    try {
      const out = p.id ? await api("PUT", "packs/" + p.id, p) : await api("POST", "packs", p);
      cur = out; setDirty(false);
      $("pf-target").value = out.target;
      return out;
    } catch (e) { toast(e.message, true); return null; }
  }

  async function writeBrief() {
    const p = await save();
    if (!p) return;
    try {
      const r = await api("GET", "packs/" + p.id + "/brief");
      $("pf-brief-text").value = r.brief;
      $("pf-copy").disabled = false;
      toast("Brief ready — copy it to Claude");
    } catch (e) { toast(e.message, true); }
  }

  async function check() {
    if (!cur || !cur.id) { const p = await save(); if (!p) return; }
    const answer = $("pf-answer").value;
    if (!answer.trim()) { toast("Paste Claude's answer first", true); return; }
    try {
      const pv = await api("POST", "packs/" + cur.id + "/preview", { answer });
      lastPreview = pv;
      renderPreview(pv);
      $("pf-import").disabled = !pv.ok;
    } catch (e) { toast(e.message, true); }
  }

  function renderPreview(pv) {
    const el = $("pf-preview");
    el.hidden = false;
    const over = pv.target && pv.duration > pv.target * 1.15;
    el.innerHTML =
      `<div class="pv-stats"><span><b>${pv.sections}</b> sections</span><span><b>${pv.qa}</b> questions</span><span><b>${pv.words}</b> words</span>` +
      `<span class="${over ? "late" : ""}"><b>${fmt(pv.duration)}</b> of ${fmt(pv.target)}</span><span class="${pv.checks ? "late" : ""}"><b>${pv.checks}</b> [check]</span></div>` +
      (pv.errors.length ? `<ul class="pv-err">${pv.errors.map((x) => "<li>" + esc(x) + "</li>").join("")}</ul>` : "") +
      (pv.warnings.length ? `<ul class="pv-warn">${pv.warnings.map((x) => "<li>" + esc(x) + "</li>").join("")}</ul>` : "") +
      (pv.sectionTitles.length ? `<details><summary>Sections</summary><ol>${pv.sectionTitles.map((t) => "<li>" + esc(t) + "</li>").join("")}</ol></details>` : "");
  }

  async function doImport() {
    if (!lastPreview || !lastPreview.ok) return check();
    try {
      const r = await api("POST", "packs/" + cur.id + "/import", { answer: $("pf-answer").value });
      cur.scriptId = r.script.id;
      $("pf-done").hidden = false;
      $("pf-import").textContent = "Update script";
      $("pf-done-msg").textContent = `“${r.script.title}” is in your library: ${r.script.parsed.paragraphs} paragraphs, ${r.script.parsed.qaItems} answers ready as cues.`;
      toast("Script created");
    } catch (e) { toast(e.message, true); }
  }

  async function openPack(id) {
    try { fill(await api("GET", "packs/" + id)); } catch (e) { return toast(e.message, true); }
    await app.show("pack");
    try {
      const r = await api("GET", "packs/" + id + "/brief");
      $("pf-brief-text").value = r.brief; $("pf-copy").disabled = false;
    } catch (_) { /* ignore */ }
  }

  function newPack() {
    fill({ title: "", occasion: "interview", lang: "en", target: "2:00", wpm: 140, qaCount: 8, answerMax: 60, topics: [], facts: [], avoid: [], sources: "" });
    app.show("pack").then(() => $("pf-title").focus());
  }

  async function open() { await list(); return app.show("packs"); }

  function init(ctx) {
    app = ctx;
    $("btn-packs").onclick = open;
    $("btn-packs-empty").onclick = newPack;
    $("pk-back").onclick = async () => { await app.loadLibrary(); app.show("library"); };
    $("pk-new").onclick = newPack;
    $("pk-import").onkeydown = (e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); $("pk-file").click(); } };
    $("pk-file").onchange = async (ev) => {
      const f = ev.target.files[0]; ev.target.value = "";
      if (!f) return;
      try {
        const r = await upload("packs/import", await f.arrayBuffer());
        toast("Imported pack “" + r.pack.title + "”" + (r.scriptId ? " with its script" : ""));
        list();
      } catch (e) { toast(e.message, true); }
    };
    $("pf-back").onclick = async () => {
      if (dirty && !(await save())) return;
      open();
    };
    $("pf-save").onclick = async () => { if (await save()) toast("Saved"); };
    $("pf-brief").onclick = writeBrief;
    $("pf-copy").onclick = async () => { toast((await copyText($("pf-brief-text").value)) ? "Brief copied" : "Select the brief and copy it", false); };
    $("pf-claude").onclick = () => api("POST", "open", { url: "https://claude.ai/new" }).catch((e) => toast(e.message, true));
    $("pf-check").onclick = check;
    $("pf-import").onclick = doImport;
    $("pf-answer").addEventListener("input", () => { $("pf-import").disabled = true; lastPreview = null; });
    $("pf-answer").addEventListener("paste", () => setTimeout(check, 50));
    $("pf-open-editor").onclick = () => cur.scriptId && app.openScript(cur.scriptId, "editor");
    $("pf-rehearse").onclick = () => cur.scriptId && app.openScript(cur.scriptId, "prompter");
    $("pf-export").onclick = () => cur.id && exportPack(cur.id);
    $("pf-title").addEventListener("input", () => setDirty(true));
    for (const f of fields) $("pf-" + f).addEventListener("input", () => setDirty(true));
  }

  window.TCPacks = { init, open, newPack, isDirty: () => dirty };
})();
