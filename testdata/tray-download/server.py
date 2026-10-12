"""Loopback-only gated synthetic media fixture for native lifecycle smoke."""
import json
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

root = Path(sys.argv[1])
media = (root / "fixture.mkv").read_bytes()
gates = {i: threading.Event() for i in range(1, 6)}
seen = set()
lock = threading.Lock()


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def do_GET(self):
        path = self.path.split("?")[0]
        if path == "/status":
            with lock:
                body = json.dumps(sorted(seen)).encode()
            self.send_response(200)
            self.end_headers()
            self.wfile.write(body)
            return
        if path.startswith("/release/"):
            gates[int(path.rsplit("/", 1)[1])].set()
            self.send_response(200)
            self.end_headers()
            return
        try:
            index = int(path[1:].split(".")[0])
            gate = gates[index]
        except (ValueError, KeyError):
            self.send_error(404)
            return
        with lock:
            seen.add(index)
        self.send_response(200)
        self.send_header("Content-Type", "video/x-matroska")
        self.send_header("Content-Length", str(len(media)))
        self.end_headers()
        try:
            self.wfile.write(media[:8192])
            self.wfile.flush()
            gate.wait(120)
            self.wfile.write(media[8192:])
        except (BrokenPipeError, ConnectionResetError, ConnectionAbortedError):
            pass


server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
(root / "port.txt").write_text(str(server.server_port))
server.serve_forever()
