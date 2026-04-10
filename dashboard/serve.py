#!/usr/bin/env python3
"""Tiny local server for the metrics dashboard.

Usage: python3 dashboard/serve.py
       → opens http://localhost:9191 in the browser

GET /           → serves index.html
GET /data.jsonl → serves collected metrics
POST /collect   → runs collect-metrics.sh and returns fresh data
"""

import http.server
import json
import os
import subprocess
import sys
import webbrowser
from pathlib import Path

PORT = 9191
DASHBOARD_DIR = Path(__file__).resolve().parent
CORE_DIR = DASHBOARD_DIR.parent
COLLECT_SCRIPT = CORE_DIR / "scripts" / "collect-metrics.sh"
DATA_FILE = DASHBOARD_DIR / "data.jsonl"


class Handler(http.server.SimpleHTTPRequestHandler):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, directory=str(DASHBOARD_DIR), **kwargs)

    def do_POST(self):
        if self.path == "/collect":
            try:
                result = subprocess.run(
                    ["bash", str(COLLECT_SCRIPT)],
                    capture_output=True,
                    text=True,
                    timeout=15,
                )
                # Return the fresh data.jsonl content
                if DATA_FILE.exists():
                    data = DATA_FILE.read_text()
                else:
                    data = ""

                self.send_response(200)
                self.send_header("Content-Type", "application/jsonl")
                self.send_header("Access-Control-Allow-Origin", "*")
                self.end_headers()
                self.wfile.write(data.encode())
            except Exception as e:
                self.send_response(500)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(json.dumps({"error": str(e)}).encode())
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, format, *args):
        # Quiet logging — only errors
        if args and "404" in str(args[0]):
            super().log_message(format, *args)


def main():
    # Run collect on startup so data is fresh
    if COLLECT_SCRIPT.exists():
        subprocess.run(["bash", str(COLLECT_SCRIPT)], capture_output=True)

    server = http.server.HTTPServer(("127.0.0.1", PORT), Handler)
    url = f"http://localhost:{PORT}"
    print(f"Dashboard running at {url}")
    webbrowser.open(url)

    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nStopped.")
        server.server_close()


if __name__ == "__main__":
    main()
