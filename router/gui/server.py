#!/usr/bin/env python3
import http.server
import json
import os
import sqlite3
import sys
import urllib.parse

DB_PATH = "/home/mrflow/Documents/FLowork_os/router/brain/flowork-brain.sqlite"
PORT = 8585

class BrainHandler(http.server.BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Suppress logging to keep console clean
        pass

    def send_json(self, data, status=200):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        self.wfile.write(json.dumps(data).encode("utf-8"))

    def send_html(self, content, status=200):
        self.send_response(status)
        self.send_header("Content-Type", "text/html; charset=utf-8")
        self.end_headers()
        self.wfile.write(content.encode("utf-8"))

    def do_GET(self):
        parsed_url = urllib.parse.urlparse(self.path)
        path = parsed_url.path
        query = urllib.parse.parse_qs(parsed_url.query)

        # Serve frontend HTML
        if path == "/" or path == "/index.html":
            try:
                with open(os.path.join(os.path.dirname(__file__), "index.html"), "r", encoding="utf-8") as f:
                    self.send_html(f.read())
            except Exception as e:
                self.send_html(f"<h1>Error loading index.html: {str(e)}</h1>", 500)
            return

        # Serve API: Stats
        if path == "/api/stats":
            try:
                conn = sqlite3.connect(DB_PATH)
                flowork_count = conn.execute(
                    "SELECT COUNT(*) FROM drawers WHERE room='flowork_instinct' AND deleted_at IS NULL"
                ).fetchone()[0]
                total_count = conn.execute(
                    "SELECT COUNT(*) FROM drawers WHERE deleted_at IS NULL"
                ).fetchone()[0]
                leaks_count = conn.execute(
                    "SELECT COUNT(*) FROM drawers WHERE room='flowork_instinct' AND (content LIKE '%PowerShell%' OR content LIKE '%powershell%') AND deleted_at IS NULL"
                ).fetchone()[0]
                conn.close()

                self.send_json({
                    "flowork_count": flowork_count,
                    "total_count": total_count,
                    "leaks_count": leaks_count,
                    "db_path": DB_PATH
                })
            except Exception as e:
                self.send_json({"error": str(e)}, 500)
            return

        # Serve API: Search / List
        if path == "/api/search":
            q = query.get("q", [""])[0].strip()
            limit = int(query.get("limit", [20])[0])
            offset = int(query.get("offset", [0])[0])

            try:
                conn = sqlite3.connect(DB_PATH)
                results = []
                
                if q:
                    # FTS Search
                    # Match query
                    cursor = conn.execute(
                        """SELECT d.id, d.content, d.wing, d.room, d.importance, d.content_hash
                           FROM memory_fts f
                           JOIN drawers d ON d.id = f.drawer_id
                           WHERE f.room = 'flowork_instinct' AND f.content MATCH ? AND d.deleted_at IS NULL
                           LIMIT ? OFFSET ?""",
                        (q, limit, offset)
                    )
                else:
                    # Regular list
                    cursor = conn.execute(
                        """SELECT id, content, wing, room, importance, content_hash
                           FROM drawers
                           WHERE room = 'flowork_instinct' AND deleted_at IS NULL
                           ORDER BY rowid DESC
                           LIMIT ? OFFSET ?""",
                        (limit, offset)
                    )

                for row in cursor:
                    results.append({
                        "id": row[0],
                        "content": row[1],
                        "wing": row[2],
                        "room": row[3],
                        "importance": row[4],
                        "content_hash": row[5]
                    })
                conn.close()
                self.send_json({"results": results, "query": q})
            except Exception as e:
                self.send_json({"error": str(e)}, 500)
            return

        # Serve API: Detail
        if path.startswith("/api/drawer/"):
            drawer_id = path.split("/")[-1]
            try:
                conn = sqlite3.connect(DB_PATH)
                row = conn.execute(
                    """SELECT id, content, wing, room, source_file, source_type, importance, content_hash, created_at
                       FROM drawers
                       WHERE id = ? AND deleted_at IS NULL LIMIT 1""",
                    (drawer_id,)
                ).fetchone()
                conn.close()

                if row:
                    self.send_json({
                        "id": row[0],
                        "content": row[1],
                        "wing": row[2],
                        "room": row[3],
                        "source_file": row[4],
                        "source_type": row[5],
                        "importance": row[6],
                        "content_hash": row[7],
                        "created_at": row[8]
                    })
                else:
                    self.send_json({"error": "Drawer not found"}, 404)
            except Exception as e:
                self.send_json({"error": str(e)}, 500)
            return

        self.send_response(404)
        self.end_headers()
        self.wfile.write(b"Not Found")

def main():
    if not os.path.exists(DB_PATH):
        print(f"Error: Database not found at {DB_PATH}", file=sys.stderr)
        sys.exit(1)
        
    server = http.server.HTTPServer(("0.0.0.0", PORT), BrainHandler)
    print(f"FLowork Brain GUI Server started at http://localhost:{PORT}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        print("\nShutting down server.")
        server.server_close()

if __name__ == "__main__":
    main()
