#!/usr/bin/env python3
"""
AllSpark Fiscal - Live Production E2E Verification Script
Tests all public and authenticated endpoints against the live production environment.
"""
import base64
import json
import os
import sys
import time
import urllib.error
import urllib.request
import uuid

BASE_URL = os.environ.get("ALLSPARK_URL", "https://allsparkfiscal.allsparkhub.com")
API_KEY = os.environ.get("ALLSPARK_API_KEY", "af_cBKACw1-xFvCspPFWzQLQmb2huLooVtWkavh4vTygE4")

GREEN = "\033[92m"
RED = "\033[91m"
YELLOW = "\033[93m"
BLUE = "\033[94m"
RESET = "\033[0m"


def log(msg, status="INFO"):
    colors = {"INFO": BLUE, "PASS": GREEN, "FAIL": RED, "WARN": YELLOW}
    print(f"{colors.get(status, RESET)}[{status}]{RESET} {msg}")


def request(method, path, body=None, token=API_KEY):
    data = json.dumps(body).encode() if body is not None else None
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(f"{BASE_URL}{path}", data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            ct = resp.headers.get("Content-Type", "")
            raw = resp.read()
            if "application/json" in ct:
                return resp.status, json.loads(raw.decode())
            return resp.status, raw
    except urllib.error.HTTPError as e:
        raw = e.read()
        try:
            return e.code, json.loads(raw.decode())
        except Exception:
            return e.code, raw


def run_checks():
    log(f"Iniciando Verificação E2E no ambiente: {BASE_URL}", "INFO")
    passed = 0
    total = 0

    def check(name, condition, details=""):
        nonlocal passed, total
        total += 1
        if condition:
            log(f"{name} -> {details}", "PASS")
            passed += 1
            return True
        else:
            log(f"{name} FAILED -> {details}", "FAIL")
            return False

    # 1. Healthz
    st, res = request("GET", "/healthz", token=None)
    data = res.get("data", {}) if isinstance(res, dict) else {}
    check("1. GET /healthz", st == 200 and data.get("status") == "ok", f"Status: {st}, Status={data.get('status')}")

    # 2. Readyz
    st, res = request("GET", "/readyz", token=None)
    data = res.get("data", {}) if isinstance(res, dict) else {}
    check("2. GET /readyz", st == 200 and data.get("ready") is True, f"Status: {st}, Ready={data.get('ready')}")

    # 3. Capabilities
    st, res = request("GET", "/v1/capabilities")
    cap = res.get("data", {}) if isinstance(res, dict) else {}
    check("3. GET /v1/capabilities", st == 200 and cap.get("mcp_server") == "/v1/mcp", f"Shield Anti-656: {cap.get('sefaz_anti_656_shield')}")

    # 4. Error Catalog (SEFAZ_656)
    st, res = request("GET", "/v1/utils/errors/SEFAZ_656")
    err_data = res.get("data", {}) if isinstance(res, dict) else {}
    check("4. GET /v1/utils/errors/SEFAZ_656", st == 200 and "Consumo Indevido" in str(err_data.get("message", "")), f"Mensagem: {err_data.get('message')}")

    # 5. Utilities (CFOP)
    st, res = request("GET", "/v1/utils/cfop")
    cfop_data = res.get("data", {}) if isinstance(res, dict) else {}
    check("5. GET /v1/utils/cfop", st == 200 and len(cfop_data.get("items", [])) > 0, f"{len(cfop_data.get('items', []))} CFOPs carregados")

    # 6. MCP Protocol Discovery
    st, res = request("GET", "/v1/mcp")
    mcp_data = res.get("data", {}) if isinstance(res, dict) else {}
    server_info = mcp_data.get("server", {})
    check("6. GET /v1/mcp (Discovery)", st == 200 and server_info.get("name") == "allspark-fiscal-mcp", f"Servidor: {server_info.get('name')} v{server_info.get('version')}")

    # 7. MCP JSON-RPC Tool Call
    mcp_call = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
            "name": "diagnose_fiscal_error",
            "arguments": {"code": "SEFAZ_656"},
        },
    }
    st, res = request("POST", "/v1/mcp", body=mcp_call)
    rpc_res = res.get("result", {}) if isinstance(res, dict) else {}
    check("7. POST /v1/mcp (JSON-RPC diagnose_fiscal_error)", st == 200 and "action" in str(rpc_res), "Diagnóstico fiscal executado via MCP")

    # 8. Document Generation & Retrieval (Order-to-Invoice)
    order_ref = f"E2E-PROD-{uuid.uuid4().hex[:6]}"
    order_payload = {
        "company_id": "1d835da5-cf42-4f2a-a5fa-9fe8fe644297",
        "ref": order_ref,
        "operation": "generate",
        "customer": {
            "name": "CLIENTE TESTE E2E",
            "tax_id": "11144477735",
            "ie_indicator": 9,
            "address": {
                "street": "Avenida Tancredo Neves",
                "number": "500",
                "district": "Caminho das Arvores",
                "city": "Salvador",
                "state": "BA",
                "postal_code": "41820021",
                "city_code": "2927408",
            },
        },
        "items": [
            {
                "sku": "SKU-E2E-001",
                "description": "NOTEBOOK PRO 16GB SSD 512GB",
                "ncm": "84713012",
                "quantity": "1.0000",
                "price": "4500.00",
            }
        ],
        "payment_method": "pix",
    }

    st, res = request("POST", "/v1/orders/invoice", body=order_payload)
    if check("8. POST /v1/orders/invoice (202 Accepted)", st == 202, f"Doc Ref: {order_ref}"):
        doc_id = res["data"]["id"]
        log(f"Aguardando processamento assíncrono (RabbitMQ -> Engine NFePHP) do documento {doc_id}...", "INFO")
        
        # Poll up to 15 seconds for worker processing
        final_doc = None
        for _ in range(15):
            time.sleep(1)
            s, r = request("GET", f"/v1/documents/ref/{order_ref}")
            if s == 200 and r.get("data", {}).get("status") in ("validated", "authorized", "failed"):
                final_doc = r["data"]
                break

        if final_doc:
            status = final_doc.get("status")
            access_key = final_doc.get("access_key")
            check("9. Processamento Assíncrono da NF-e", status == "validated", f"Status: {status}, Chave SEFAZ: {access_key}")

            # 10. Download XML
            st_xml, xml_data = request("GET", f"/v1/documents/{doc_id}/xml")
            has_xml_sig = b"<Signature" in xml_data if isinstance(xml_data, bytes) else "<Signature" in xml_data
            check("10. GET /v1/documents/{id}/xml", st_xml == 200 and has_xml_sig, f"XML Assinado ({len(xml_data)} bytes com XMLDSig)")

            # 11. Download PDF
            st_pdf, pdf_data = request("GET", f"/v1/documents/{doc_id}/pdf")
            is_valid_pdf = isinstance(pdf_data, bytes) and pdf_data.startswith(b"%PDF")
            check("11. GET /v1/documents/{id}/pdf", st_pdf == 200 and is_valid_pdf, f"DANFE PDF Válido ({len(pdf_data)} bytes)")
        else:
            check("9. Processamento Assíncrono da NF-e", False, "Timeout aguardando processamento do worker")

    print("\n" + "=" * 65)
    log(f"Resultado Final: {passed}/{total} testes passaram com 100% de sucesso!", "PASS" if passed == total else "WARN")
    print("=" * 65 + "\n")
    return passed == total


if __name__ == "__main__":
    success = run_checks()
    sys.exit(0 if success else 1)
