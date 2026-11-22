// Phone sync dialog: the PC is the hub; pair the Android app with a 6-digit code comparison.
(function () {
  "use strict";
  const { $, api, toast, esc, ago } = window.TC;
  let st = null, poll = 0, bg = 0, shownFor = "";

  function render() {
    if (!st) return;
    $("sy-on").checked = st.enabled;
    const b = $("sy-body");
    let h = "";
    if (st.error) h += `<p class="warn">${esc(st.error)}</p>`;
    if (st.enabled && st.running) {
      h += `<p>Phones find this PC by itself on the same network. If not, type one of these addresses in the app:</p>` +
        `<p class="sy-addr">${st.addresses.map(esc).join(" · ") || "no network found"}</p>` +
        `<p class="hint">PC “${esc(st.pcName)}” · identity ${esc(st.fingerprint)}${st.lastSync ? " · last sync " + ago(st.lastSync) : ""}</p>`;
    } else if (st.enabled) {
      h += `<p class="hint">Starting…</p>`;
    } else {
      h += `<p class="hint">Sync is off. Phones keep working on their own.</p>`;
    }
    const p = st.pending;
    if (p && p.state === "compare") {
      h += `<div class="sy-pending"><p><b>${esc(p.phoneName)}</b> wants to pair. Does the phone show this code?</p>` +
        `<div class="sy-code">${esc(p.code.slice(0, 3))} ${esc(p.code.slice(3))}</div>` +
        `<div class="actions"><button id="sy-reject">No, cancel</button><button id="sy-accept" class="primary">Yes, it matches</button></div></div>`;
    } else if (st.pairing) {
      h += `<div class="sy-pending"><p>Ready to pair. On the phone open <b>TeleCyborg → Sync → Pair with PC</b> and choose this PC.</p>` +
        `<p class="hint">Open until ${new Date(st.pairUntil).toLocaleTimeString()} · <button id="sy-cancel" class="mini">Stop</button></p></div>`;
    }
    h += `<h3>Paired phones</h3>` + (st.phones.length
      ? `<ul class="sy-phones">${st.phones.map((ph) => `<li><b>${esc(ph.name)}</b><span>paired ${new Date(ph.paired_at).toLocaleDateString()} · last sync ${ago(ph.last_sync)}</span><button class="mini" data-forget="${esc(ph.id)}">Forget</button></li>`).join("")}</ul>`
      : `<p class="hint">None yet.</p>`);
    b.innerHTML = h;
    $("sy-pair").hidden = st.pairing || (p && p.state === "compare");
    const acc = $("sy-accept"), rej = $("sy-reject"), can = $("sy-cancel");
    if (acc) acc.onclick = () => decide(true);
    if (rej) rej.onclick = () => decide(false);
    if (can) can.onclick = async () => { st = await api("POST", "hub/pair/cancel"); render(); };
    b.querySelectorAll("[data-forget]").forEach((el) => (el.onclick = async () => {
      st = await api("POST", "hub/forget", { id: el.dataset.forget }); render(); toast("Phone forgotten");
    }));
  }
  async function refresh() { try { st = await api("GET", "hub/status"); render(); } catch (e) { $("sy-msg").textContent = e.message; } }
  async function decide(ok) {
    try { st = await api("POST", "hub/decide", { ok }); render(); toast(ok ? "Phone paired" : "Pairing cancelled"); }
    catch (e) { toast(e.message, true); }
    setTimeout(refresh, 600);
  }
  function open() {
    $("sync-modal").hidden = false; $("sy-msg").textContent = "";
    refresh(); clearInterval(poll); poll = setInterval(refresh, 1000);
  }
  function close() { $("sync-modal").hidden = true; clearInterval(poll); }

  function init() {
    $("btn-sync").onclick = open;
    $("sy-close").onclick = close;
    $("sync-modal").addEventListener("click", (e) => { if (e.target.id === "sync-modal") close(); });
    $("sync-modal").addEventListener("keydown", (e) => { if (e.key === "Escape") { e.stopPropagation(); close(); } });
    $("sy-on").onchange = async () => {
      try { st = await api("POST", "hub/enable", { on: $("sy-on").checked }); render(); }
      catch (e) { $("sy-msg").textContent = e.message; refresh(); }
    };
    $("sy-pair").onclick = async () => {
      try { st = await api("POST", "hub/pair"); render(); } catch (e) { $("sy-msg").textContent = e.message; }
    };
    // a phone may start pairing while the dialog is closed: pop it up
    bg = setInterval(async () => {
      if (!$("sync-modal").hidden) return;
      let s; try { s = await api("GET", "hub/status"); } catch (_) { return; }
      if (s.pending && s.pending.state === "compare" && shownFor !== s.pending.id) { shownFor = s.pending.id; open(); }
    }, 2500);
  }
  window.TCSync = { init, open, isOpen: () => !$("sync-modal").hidden };
})();
