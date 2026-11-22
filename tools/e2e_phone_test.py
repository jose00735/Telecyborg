"""End-to-end test of v1.1 on the PC: phone-sync pairing from the UI, the camera self-view and
the Macawlink phone feed (through a real Macawlink receiver, `devreceiver`, and a fake phone).

    python3 tools/e2e_phone_test.py <telecyborg-binary> <macawlink-devreceiver-binary>
"""
import json, os, re, ssl, subprocess, sys, tempfile, threading, time, urllib.request, pathlib, hashlib
from playwright.sync_api import sync_playwright, expect

SHOTS = pathlib.Path(os.environ.get("SHOTS", "/tmp/tc-shots")); SHOTS.mkdir(exist_ok=True)


def lines_of(proc, sink):
    for line in proc.stdout:
        sink.append(line.rstrip())


def phone_call(path, body, ctx, timeout=10):
    req = urllib.request.Request("https://127.0.0.1:47811" + path, data=json.dumps(body).encode(), method="POST",
                                 headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, context=ctx, timeout=timeout) as r:
        return json.loads(r.read())


def main():
    tc, mk = sys.argv[1], sys.argv[2]
    tmp = tempfile.mkdtemp()
    mk_out = []
    mkp = subprocess.Popen([mk, "-tap", "next"], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True)
    threading.Thread(target=lines_of, args=(mkp, mk_out), daemon=True).start()
    app = subprocess.Popen([tc, "-no-open", "-data", os.path.join(tmp, "data")], stdout=subprocess.PIPE, text=True, env=dict(os.environ, HOME=tmp))
    url = None
    for _ in range(200):
        m = re.search(r"(http://127\.0\.0\.1:\d+/#t=[0-9a-f]+)", app.stdout.readline())
        if m: url = m.group(1); break
    assert url
    errors = []
    try:
        with sync_playwright() as p:
            b = p.chromium.launch(executable_path="/usr/bin/google-chrome", args=["--use-fake-ui-for-media-stream", "--use-fake-device-for-media-stream"])
            pg = b.new_page(viewport={"width": 1280, "height": 800})
            pg.on("pageerror", lambda e: errors.append(str(e)))
            pg.goto(url)
            expect(pg.locator("#screen-library")).to_be_visible(timeout=10000)
            pg.click("#btn-motion")

            # ---- phone sync: enable, open pairing, a "phone" pairs, the PC user accepts ----
            pg.click("#btn-sync")
            expect(pg.locator("#sync-modal")).to_be_visible()
            pg.click("#sy-pair")
            expect(pg.locator("#sy-body")).to_contain_text("Ready to pair", timeout=5000)
            expect(pg.locator("#sy-body")).to_contain_text(":47811")
            ctx = ssl.create_default_context(); ctx.check_hostname = False; ctx.verify_mode = ssl.CERT_NONE
            begin = phone_call("/v1/pair/begin", {"phone_id": "py-phone", "name": "Test Pixel"}, ctx)
            nonce = os.urandom(32).hex()
            phone_call("/v1/pair/nonce", {"pair_id": begin["pair_id"], "nonce": nonce}, ctx)
            expect(pg.locator(".sy-code")).to_be_visible(timeout=5000)
            pg.screenshot(path=str(SHOTS / "10-sync-pairing.png"))
            result = {}
            t = threading.Thread(target=lambda: result.update(phone_call("/v1/pair/confirm", {"pair_id": begin["pair_id"], "ok": True}, ctx, timeout=30)))
            t.start(); time.sleep(0.5)
            pg.click("#sy-accept")
            t.join(10)
            assert result.get("ok") and result.get("token"), result
            expect(pg.locator(".sy-phones")).to_contain_text("Test Pixel", timeout=5000)
            pg.screenshot(path=str(SHOTS / "11-sync-paired.png"))
            pg.click("#sy-close")

            # ---- sample script → prompter ----
            pg.click("#btn-sample")
            expect(pg.locator("#screen-prompter")).to_be_visible()
            pg.wait_for_timeout(3600)
            # camera self-view
            pg.keyboard.press("v")
            expect(pg.locator("#pr-view.camview")).to_be_visible(timeout=5000)
            expect(pg.locator("#pr-video")).to_be_visible()
            pg.wait_for_timeout(800)
            pg.screenshot(path=str(SHOTS / "12-camera-view.png"))
            pg.keyboard.press("v")
            expect(pg.locator("#pr-view.camview")).to_have_count(0)

            # ---- Macawlink phone feed: script + positions reach the phone, a tap comes back ----
            before = pg.locator("#pr-content .para.now").get_attribute("data-i")
            pg.keyboard.press("p")
            expect(pg.locator("#pr-phone.on")).to_be_visible(timeout=5000)
            for _ in range(60):
                if any(l.startswith("SCRIPT") for l in mk_out) and sum(l.startswith("STATE") for l in mk_out) >= 5: break
                time.sleep(0.2)
            scripts = [l for l in mk_out if l.startswith("SCRIPT")]
            states = [l for l in mk_out if l.startswith("STATE")]
            assert scripts and len(states) >= 5, mk_out[-5:]
            sc = json.loads(scripts[0][7:])
            assert sc["title"].startswith("Sample") and any(b["k"] == "para" for b in sc["blocks"]), sc
            st = json.loads(states[-1][6:])
            assert {"para", "frac", "speed", "playing", "clock"} <= set(st), st
            # the fake phone tapped "next": the prompter on the PC moved on
            expect(pg.locator("#pr-content .para.now")).not_to_have_attribute("data-i", before or "0", timeout=5000)
            pg.screenshot(path=str(SHOTS / "13-phone-feed.png"))
            pg.keyboard.press("Escape")
            expect(pg.locator("#screen-report")).to_be_visible(timeout=8000)
            for _ in range(30):
                if "CLEAR" in mk_out: break
                time.sleep(0.2)
            assert "CLEAR" in mk_out, "phone prompter not cleared at the end"
            b.close()
    finally:
        app.terminate(); mkp.terminate()
    if errors:
        print("page errors:", *errors, sep="\n  "); sys.exit(1)
    print("e2e phone ok")


if __name__ == "__main__":
    main()
