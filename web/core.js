// Shared helpers for the TeleCyborg UI: API access with the per-launch token, formatting, toasts.
(function () {
  "use strict";
  const $ = (id) => document.getElementById(id);

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
    if (!res.ok) { const e = new Error(data.error || "Request failed (" + res.status + ")"); e.status = res.status; e.data = data; throw e; }
    return data;
  }
  // binary upload (audio chunks, model files, pack zips)
  async function upload(path, data, signal) {
    const res = await fetch("/api/" + path, {
      method: "POST", headers: { "X-TC-Token": token, "Content-Type": "application/octet-stream" }, body: data, signal,
    });
    const out = await res.json().catch(() => ({}));
    if (!res.ok) { const e = new Error(out.error || "Request failed (" + res.status + ")"); e.status = res.status; throw e; }
    return out;
  }

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
    clearTimeout(toastTimer); toastTimer = setTimeout(() => (t.hidden = true), bad ? 6500 : 2600);
  }
  const esc = (s) => String(s == null ? "" : s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));
  // accent- and case-insensitive text for search
  const fold = (s) => String(s || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

  async function copyText(text) {
    try { await navigator.clipboard.writeText(text); return true; }
    catch (_) {
      const ta = document.createElement("textarea");
      ta.value = text; ta.style.position = "fixed"; ta.style.opacity = "0";
      document.body.appendChild(ta); ta.select();
      let ok = false; try { ok = document.execCommand("copy"); } catch (_) { /* ignore */ }
      ta.remove(); return ok;
    }
  }

  window.TC = { $, api, upload, fmt, parseDur, ago, toast, esc, fold, copyText, get token() { return token; } };
})();
