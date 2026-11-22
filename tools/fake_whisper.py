"""A stand-in for whisper.cpp's whisper-server, for UI tests without a speech model.

The speaker side (requests whose prompt starts with a hesitation lead) gets the next words of
SCRIPT; the other side gets QUESTION once every few requests."""
import http.server, sys, json, re

SCRIPT = sys.argv[2] if len(sys.argv) > 2 else ""
QUESTION = "So, what would you say is your biggest weakness?"
state = {"me": 0, "other": 0}

class H(http.server.BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
    def do_POST(self):
        n = int(self.headers.get("Content-Length", 0)); body = self.rfile.read(n).decode("latin-1")
        m = re.search(r'name="prompt"\r\n\r\n(.*?)\r\n--', body, re.S)
        prompt = m.group(1) if m else ""
        if prompt.startswith(("Um,", "Eh,", "Hum,")):
            words = SCRIPT.split(); i = state["me"]; state["me"] += 5
            text = " ".join(words[i:i + 7]) if i < len(words) else ""
            if i == 10: text = "um " + text
        else:
            state["other"] += 1
            text = QUESTION if state["other"] % 3 == 1 else "okay, great"
        out = json.dumps({"text": " " + text}).encode()
        self.send_response(200); self.send_header("Content-Type", "application/json"); self.end_headers(); self.wfile.write(out)

http.server.ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
