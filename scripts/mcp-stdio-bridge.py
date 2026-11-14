#!/usr/bin/env python3
"""AllSpark Fiscal MCP STDIO-to-HTTP Bridge.

Permite conectar clientes MCP locais que utilizam transporte STDIO
(ex.: Claude Desktop, Cursor, Windsurf, Gemini CLI, Zed) diretamente ao
endpoint seguro /v1/mcp da AllSpark Fiscal.

Variáveis de ambiente exigidas:
  ALLSPARK_API_URL: URL base da API (padrão: http://127.0.0.1:8088)
  ALLSPARK_API_KEY: Chave Bearer do tenant (ex.: af_live_...)
"""
import json
import os
import sys
import urllib.error
import urllib.request

API_URL = os.environ.get("ALLSPARK_API_URL", "http://127.0.0.1:8088").rstrip("/")
API_KEY = os.environ.get("ALLSPARK_API_KEY", "")


def forward_rpc(payload: dict) -> dict | None:
    # JSON-RPC notifications (no 'id') do not expect a response frame in STDIO
    is_notification = "id" not in payload or payload.get("id") is None
    data = json.dumps(payload).encode("utf-8")
    headers = {
        "Content-Type": "application/json",
        "Authorization": f"Bearer {API_KEY}",
    }
    req = urllib.request.Request(f"{API_URL}/v1/mcp", data=data, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read().decode("utf-8")
            if is_notification:
                return None
            return json.loads(raw)
    except urllib.error.HTTPError as e:
        if is_notification:
            return None
        err_body = e.read().decode("utf-8", errors="replace")
        return {
            "jsonrpc": "2.0",
            "id": payload.get("id"),
            "error": {
                "code": -32000,
                "message": f"HTTP {e.code} from AllSpark Fiscal MCP",
                "data": err_body,
            },
        }
    except Exception as exc:
        if is_notification:
            return None
        return {
            "jsonrpc": "2.0",
            "id": payload.get("id"),
            "error": {
                "code": -32603,
                "message": f"AllSpark MCP Bridge connection error: {exc}",
            },
        }


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            continue
        reply = forward_rpc(msg)
        if reply is not None:
            sys.stdout.write(json.dumps(reply, ensure_ascii=False) + "\n")
            sys.stdout.flush()


if __name__ == "__main__":
    main()

