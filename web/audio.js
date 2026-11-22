// Audio capture for listening (stage 2) and smart follow (stage 3).
// Captures a microphone / input device or the system audio (screen-share audio), resamples to
// 16 kHz mono, detects speech with an adaptive energy gate and cuts it into chunks for the local
// speech engine. Audio stays in memory; nothing is recorded to disk (PV-3).
(function () {
  "use strict";
  const RATE = 16000;

  async function devices() {
    if (!navigator.mediaDevices || !navigator.mediaDevices.enumerateDevices) return [];
    const all = await navigator.mediaDevices.enumerateDevices();
    return all.filter((d) => d.kind === "audioinput").map((d, i) => ({ id: d.deviceId, label: d.label || "Input " + (i + 1) }));
  }

  // Ask once for the microphone so device names become visible.
  async function unlockLabels() {
    const s = await navigator.mediaDevices.getUserMedia({ audio: true });
    s.getTracks().forEach((t) => t.stop());
  }

  async function openStream(source) {
    if (!navigator.mediaDevices) throw new Error("Audio capture is not available here.");
    if (source.kind === "system") {
      if (!navigator.mediaDevices.getDisplayMedia) throw new Error("This window cannot capture system audio. Choose an input device instead (for example Stereo Mix or a virtual cable).");
      const s = await navigator.mediaDevices.getDisplayMedia({
        video: true, audio: { echoCancellation: false, noiseSuppression: false, autoGainControl: false }, systemAudio: "include",
      });
      if (!s.getAudioTracks().length) {
        s.getTracks().forEach((t) => t.stop());
        throw new Error("No audio was shared. Share your entire screen and tick “Share system audio”.");
      }
      // keep the video track alive (some browsers end the share without it) but do not use it
      s.getVideoTracks().forEach((t) => (t.enabled = false));
      return s;
    }
    // voice processing helps a microphone but damages a loopback device (Stereo Mix, virtual cable)
    const voice = source.kind === "mic";
    const audio = { echoCancellation: voice, noiseSuppression: voice, autoGainControl: voice, channelCount: 1 };
    if (source.deviceId) audio.deviceId = { exact: source.deviceId };
    return navigator.mediaDevices.getUserMedia({ audio });
  }

  // Source: one capture with a speech chunker on top.
  // opts: { maxSec, silenceMs, overlapSec, onChunk(Int16Array, info), onSpeaking(bool), onLevel(0..1), onEnded() }
  async function capture(source, opts) {
    const stream = await openStream(source);
    const Ctx = window.AudioContext || window.webkitAudioContext;
    const ctx = new Ctx();
    const input = ctx.createMediaStreamSource(stream);
    const chunker = new Chunker(ctx.sampleRate, opts);
    let node;
    try {
      await ctx.audioWorklet.addModule("pcm-worklet.js");
      node = new AudioWorkletNode(ctx, "pcm-tap");
      node.port.onmessage = (e) => chunker.push(e.data);
    } catch (_) {
      node = ctx.createScriptProcessor(2048, 1, 1);
      node.onaudioprocess = (e) => chunker.push(new Float32Array(e.inputBuffer.getChannelData(0)));
    }
    const mute = ctx.createGain(); mute.gain.value = 0;
    input.connect(node); node.connect(mute); mute.connect(ctx.destination);
    let stopped = false;
    stream.getTracks().forEach((t) => t.addEventListener("ended", () => { if (!stopped && opts.onEnded) opts.onEnded(); }));
    if (ctx.state === "suspended") ctx.resume().catch(() => {});
    return {
      label: source.kind === "system" ? "System audio" : (stream.getAudioTracks()[0] || {}).label || "Microphone",
      chunker,
      stop() {
        stopped = true;
        chunker.flush();
        try { input.disconnect(); node.disconnect(); } catch (_) { /* ignore */ }
        stream.getTracks().forEach((t) => t.stop());
        ctx.close().catch(() => {});
      },
    };
  }

  // Record a few seconds for the engine test.
  async function record(source, seconds) {
    const parts = [];
    const src = await capture(source, { maxSec: 60, silenceMs: 1e9, onChunk() {} });
    src.chunker.raw = (pcm) => parts.push(pcm);
    await new Promise((r) => setTimeout(r, seconds * 1000));
    src.stop();
    let n = 0; parts.forEach((p) => (n += p.length));
    const out = new Int16Array(n); let o = 0;
    parts.forEach((p) => { out.set(p, o); o += p.length; });
    return out;
  }

  class Chunker {
    constructor(inRate, opts) {
      this.ratio = inRate / RATE;
      this.frac = 0;
      this.last = 0;
      this.opts = Object.assign({ maxSec: 8, silenceMs: 600, overlapSec: 0, preRollSec: 0.25, minSec: 0.5 }, opts);
      this.frame = new Float32Array(320); // 20 ms at 16 kHz
      this.fn = 0;
      this.floor = 0.004;
      this.speaking = false;
      this.hang = 0;
      this.cur = []; // Float32 frames of the chunk being built
      this.pre = []; // pre-roll frames
      this.silentFrames = 0;
      this.raw = null; // optional tap of all 16 kHz PCM (engine test)
      this.paused = false;
    }
    // linear-interpolation resampler from the context rate to 16 kHz
    push(block) {
      const r = this.ratio;
      let pos = this.frac;
      const outLen = Math.max(0, Math.floor((block.length - 1 - pos) / r) + 1);
      const out = new Float32Array(outLen);
      for (let i = 0; i < outLen; i++) {
        const x = pos + i * r;
        const i0 = Math.floor(x), t = x - i0;
        const a = i0 < 0 ? this.last : block[i0];
        const b = i0 + 1 < block.length ? block[i0 + 1] : a;
        out[i] = a + (b - a) * t;
      }
      this.frac = pos + out.length * r - block.length;
      this.last = block[block.length - 1];
      if (this.raw) this.raw(toInt16(out));
      for (let i = 0; i < out.length; i++) {
        this.frame[this.fn++] = out[i];
        if (this.fn === 320) { this.onFrame(this.frame.slice(0)); this.fn = 0; }
      }
    }
    onFrame(f) {
      let e = 0;
      for (let i = 0; i < f.length; i++) e += f[i] * f[i];
      const rms = Math.sqrt(e / f.length);
      // adaptive noise floor: falls fast, rises slowly
      this.floor = rms < this.floor ? this.floor * 0.9 + rms * 0.1 : this.floor * 0.9995 + rms * 0.0005;
      const voiced = rms > Math.max(0.01, this.floor * 3.2);
      if (this.opts.onLevel) this.opts.onLevel(Math.min(1, rms * 8));
      if (voiced) { this.hang = 15; } else if (this.hang > 0) this.hang--;
      const speech = voiced || this.hang > 0;
      if (this.paused) { this.cur = []; this.pre = []; return; }
      if (speech !== this.speaking) {
        this.speaking = speech;
        if (this.opts.onSpeaking) this.opts.onSpeaking(speech);
      }
      if (speech || this.cur.length) {
        if (!this.cur.length) this.cur = this.pre.slice();
        this.cur.push(f);
        this.silentFrames = speech ? 0 : this.silentFrames + 1;
        const len = this.cur.length * 0.02;
        if (this.silentFrames * 20 >= this.opts.silenceMs) this.emit(false);
        else if (len >= this.opts.maxSec) this.emit(true);
      } else {
        this.pre.push(f);
        const keep = Math.round(this.opts.preRollSec / 0.02);
        if (this.pre.length > keep) this.pre.shift();
      }
    }
    emit(cut) {
      const frames = this.cur;
      this.cur = [];
      this.silentFrames = 0;
      const voicedSec = frames.length * 0.02;
      if (cut && this.opts.overlapSec > 0) {
        const keep = Math.round(this.opts.overlapSec / 0.02);
        this.cur = frames.slice(-keep);
      }
      if (voicedSec < this.opts.minSec) return;
      const pcm = new Float32Array(frames.length * 320);
      frames.forEach((fr, i) => pcm.set(fr, i * 320));
      this.opts.onChunk(toInt16(pcm), { seconds: voicedSec, cut });
    }
    flush() { if (this.cur.length) this.emit(false); }
  }

  function toInt16(f) {
    const out = new Int16Array(f.length);
    for (let i = 0; i < f.length; i++) {
      const v = Math.max(-1, Math.min(1, f[i]));
      out[i] = v < 0 ? v * 0x8000 : v * 0x7fff;
    }
    return out;
  }

  window.TCAudio = { devices, unlockLabels, capture, record, RATE, _Chunker: Chunker };
})();
