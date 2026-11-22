"""End-to-end UI test: content pack -> script -> listening with suggestions -> Q&A detour and
return -> smart follow -> report with notes. Uses Chrome's fake microphone and tools/fake_whisper.py.

    python3 tools/e2e_test.py [path-to-telecyborg-binary]
"""
import os, re, subprocess, sys, tempfile, time, wave, struct, math, pathlib
from playwright.sync_api import sync_playwright, expect

ROOT = pathlib.Path(__file__).resolve().parent.parent
SHOTS = pathlib.Path(os.environ.get("SHOTS", "/tmp/tc-shots")); SHOTS.mkdir(exist_ok=True)

PITCH = [
    "Hi, I'm Jose Acosta, a Customer Success Manager based in Colombia who works in English, Spanish and Portuguese.",
    "At Netskope I moved an account from below fifty to seventy percent utilisation by bringing in the right people for a secure rollout.",
    "Before that I spent more than two years at Fortinet as a support engineer, the escalation point for enterprise cases in North America and Brazil.",
    "That mix of technical depth and customer care is what I want to bring to your team.",
]
ANSWER = """Here you go:

```markdown
---
title: Terrific interview
target: 0:40
wpm: 140
lang: en
---
# Opening
keywords: introduce yourself, about you, background
%s

# Netskope
keywords: netskope, utilisation/adoption, rollout
%s

# Fortinet
keywords: fortinet, support, escalation
%s

[pause 1]

# Close
keywords: why you, fit
%s

=== QA ===
# Q: What is your biggest weakness?
keywords: weakness, improve
I over-prepare. Now I time-box preparation and rehearse with a timer, which is why I built this teleprompter.

# Q: Why do you want this role?
keywords: why this role, motivation
Because it joins the support depth I built at Fortinet with the customer outcomes I drove at Netskope [check].
```
""" % tuple(PITCH)


def wav(path):
    r = 48000
    with wave.open(path, "w") as w:
        w.setnchannels(1); w.setsampwidth(2); w.setframerate(r)
        fr = bytearray()
        for i in range(r * 40):
            t = i / r
            v = 0.3 * math.sin(2 * math.pi * 300 * t) if (t % 2.8) < 2.2 else 0
            fr += struct.pack("<h", int(v * 32767))
        w.writeframes(bytes(fr))


def main():
    binary = sys.argv[1] if len(sys.argv) > 1 else None
    tmp = tempfile.mkdtemp()
    audio = os.path.join(tmp, "bursts.wav"); wav(audio)
    fake = subprocess.Popen([sys.executable, str(ROOT / "tools/fake_whisper.py"), "18733", " ".join(PITCH)])
    env = dict(os.environ, TELECYBORG_WHISPER_URL="http://127.0.0.1:18733", HOME=tmp)
    cmd = [binary] if binary else ["go", "run", "."]
    app = subprocess.Popen(cmd + ["-no-open", "-data", os.path.join(tmp, "data")], cwd=ROOT, env=env, stdout=subprocess.PIPE, text=True)
    url = None
    for _ in range(600):
        line = app.stdout.readline()
        m = re.search(r"(http://127\.0\.0\.1:\d+/#t=[0-9a-f]+)", line)
        if m: url = m.group(1); break
    assert url, "app did not start"
    errors = []
    try:
        with sync_playwright() as p:
            b = p.chromium.launch(executable_path="/usr/bin/google-chrome", args=[
                "--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream",
                f"--use-file-for-fake-audio-capture={audio}", "--autoplay-policy=no-user-gesture-required"])
            pg = b.new_page(viewport={"width": 1280, "height": 800})
            pg.on("pageerror", lambda e: errors.append(str(e)))
            pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
            pg.goto(url)
            expect(pg.locator("#screen-library")).to_be_visible(timeout=10000)
            pg.click("#btn-motion")  # animations off: faster test
            # ---- content pack ----
            pg.click("#btn-packs-empty")
            expect(pg.locator("#screen-pack")).to_be_visible()
            pg.fill("#pf-title", "Terrific interview")
            pg.fill("#pf-target", "0:40")
            pg.fill("#pf-qa", "2")
            pg.fill("#pf-topics", "Netskope results\nFortinet support")
            pg.fill("#pf-facts", "Moved an account from below 50% to 70% utilisation")
            pg.click("#pf-brief")
            expect(pg.locator("#pf-brief-text")).to_have_value(re.compile("=== QA ==="), timeout=5000)
            pg.fill("#pf-answer", ANSWER)
            pg.click("#pf-check")
            expect(pg.locator("#pf-preview")).to_contain_text("questions")
            pg.screenshot(path=str(SHOTS / "1-pack.png"))
            pg.click("#pf-import")
            expect(pg.locator("#pf-done")).to_be_visible()
            pg.click("#pf-open-editor")
            expect(pg.locator("#st-qa")).to_have_text("2", timeout=5000)
            expect(pg.locator("#st-checks")).to_have_text("1")
            pg.screenshot(path=str(SHOTS / "2-editor.png"))
            pg.click("#ed-rehearse")
            expect(pg.locator("#screen-prompter")).to_be_visible()
            pg.wait_for_timeout(3600)  # countdown
            # ---- listening ----
            pg.keyboard.press("l")
            expect(pg.locator("#live-modal")).to_be_visible()
            expect(pg.locator("#lm-engine")).to_contain_text("external whisper server", timeout=5000)
            pg.check("#lm-consent-ok")
            pg.select_option("#lm-other", "device")
            pg.select_option("#lm-mode", "follow")
            pg.screenshot(path=str(SHOTS / "3-listen-setup.png"))
            pg.click("#lm-start")
            expect(pg.locator("#pr-listening")).to_be_visible(timeout=8000)
            # suggestion from the other side
            expect(pg.locator("#pr-suggest li").first).to_contain_text("weakness", timeout=20000)
            pg.screenshot(path=str(SHOTS / "4-suggestion.png"))
            # follow: the voice moves the narrative forward
            expect(pg.locator("#pr-content .para.now")).to_have_attribute("data-i", re.compile("[1-9]"), timeout=25000)
            pg.keyboard.press("1")
            expect(pg.locator("#pr-answer")).to_be_visible()
            expect(pg.locator("#pr-answer-head")).to_contain_text("Q: What is your biggest weakness?")
            expect(pg.locator("#pr-crumbs")).to_contain_text("Narrative")
            pg.wait_for_timeout(1500)
            pg.screenshot(path=str(SHOTS / "5-detour.png"))
            pg.keyboard.press("Backspace")
            expect(pg.locator("#pr-content")).to_be_visible()
            expect(pg.locator("#pr-answer")).to_be_hidden()
            # Ctrl+K search
            pg.keyboard.press("Control+k")
            expect(pg.locator("#search-modal")).to_be_visible()
            pg.keyboard.type("motivation")
            expect(pg.locator("#sm-list li.sel")).to_contain_text("Why do you want this role")
            pg.keyboard.press("Enter")
            expect(pg.locator("#pr-answer-head")).to_contain_text("Why do you want this role")
            pg.keyboard.press("Backspace")
            pg.wait_for_timeout(4000)
            pg.screenshot(path=str(SHOTS / "6-follow.png"))
            pg.keyboard.press("Escape")
            expect(pg.locator("#screen-report")).to_be_visible(timeout=8000)
            expect(pg.locator("#rp-notes")).to_contain_text("biggest weakness")
            expect(pg.locator("#rp-extra")).to_contain_text("Answers")
            pg.screenshot(path=str(SHOTS / "7-report.png"), full_page=True)
            # ---- library shows the pack tag; engine dialog opens ----
            pg.click("#rp-library")
            expect(pg.locator("#script-list .tag").first).to_have_text("pack")
            pg.click("#btn-engine")
            expect(pg.locator("#lm-title")).to_have_text("Speech engine")
            pg.keyboard.press("Escape")
            b.close()
    finally:
        app.terminate(); fake.terminate()
    real = [e for e in errors if "favicon" not in e]
    if real:
        print("console errors:", *real, sep="\n  ")
        sys.exit(1)
    print("e2e ok; screenshots in", SHOTS)


if __name__ == "__main__":
    main()
