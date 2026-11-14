#!/usr/bin/env python3
"""Integration test for scalable features:
- Actionable Error Catalog & Lookup (/v1/utils/errors)
- Auxiliary Lookups (/v1/utils/cfop, /ncm, /cep, /cnpj)
- Company with logo_base64 & contingency_mode
- Certificate Expiration Monitor (/v1/certificates/expiring)
- Tax Profiles (/v1/companies/{id}/tax-profiles)
- Webhooks (/v1/webhooks & per-request webhook_url/webhook_secret)
- Beginner Order-to-Invoice (/v1/orders/invoice)
- External Reference routes (/v1/documents/ref/{ref}, /xml, /pdf, /webhook/resend)
- Monthly Accountant ZIP Backup (/v1/companies/{id}/backups/{YYYY-MM}.zip)
- Built-in MCP Server (/v1/mcp)
"""
import base64
import io
import json
import os
import pathlib
import subprocess
import tempfile
import time
import uuid
import zipfile

from integration import ROOT, call, cmd, sql, tenant

# 1x1 valid PNG in base64 for testing logo_base64 on DANFE/DANFCE
SAMPLE_PNG_B64 = (
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
)


def main():
    t = tenant("scalable-features-" + uuid.uuid4().hex[:8])
    token = t["api_key"]

    # 1. Capabilities
    status, res = call("GET", "/v1/capabilities", token)
    assert status == 200, (status, res)
    cap = res["data"]
    assert cap.get("mcp_server") == "/v1/mcp", cap
    assert cap.get("sefaz_anti_656_shield") is True, cap
    assert cap.get("webhooks") is True, cap
    assert cap.get("worker_concurrency") is True, cap

    # 2. Actionable Error Catalog & Lookup
    status, res = call("GET", "/v1/utils/errors", token)
    assert status == 200 and len(res["data"]["items"]) >= 10, res
    status, res = call("GET", "/v1/utils/errors/SEFAZ_539", token)
    assert status == 200, res
    err_539 = res["data"]
    assert "Duplicidade" in err_539["message"] and len(err_539["action"]) > 20, err_539

    # 3. Auxiliary Utilities (CFOP, NCM, CEP, CNPJ)
    status, res = call("GET", "/v1/utils/cfop", token)
    assert status == 200 and len(res["data"]["items"]) >= 5, res
    status, res = call("GET", "/v1/utils/ncm", token)
    assert status == 200 and len(res["data"]["items"]) >= 5, res
    status, res = call("GET", "/v1/utils/cep/41820-021", token)
    assert status == 200 and res["data"]["postal_code"] == "41820021" and res["data"]["valid_format"] is True, res
    status, res = call("GET", "/v1/utils/cnpj/11222333000181", token)
    assert status == 200 and res["data"]["valid_checksum"] is True, res

    # 4. Create Company with logo_base64 and contingency_mode
    comp_payload = json.loads((ROOT / "examples/company.json").read_text())
    comp_payload["logo_base64"] = SAMPLE_PNG_B64
    comp_payload["contingency_mode"] = "auto"
    status, res = call("POST", "/v1/companies", token, comp_payload)
    assert status == 201, (status, res)
    company = res["data"]
    company_id = company["id"]
    assert company.get("logo_base64") == SAMPLE_PNG_B64, company
    assert company.get("contingency_mode") == "auto", company

    # Configure NFe fiscal config
    status, res = call(
        "PUT",
        f"/v1/companies/{company_id}/fiscal/nfe",
        token,
        {"environment": "homologation", "series": 1, "enabled": True},
    )
    assert status == 200, (status, res)

    # 5. Tax Profiles
    status, res = call(
        "POST",
        f"/v1/companies/{company_id}/tax-profiles",
        token,
        {
            "name": "REVENDA_PADRAO",
            "description": "Revenda Simples Nacional com Reforma Tributaria",
            "ncm_default": "84713012",
            "cfop_internal": "5102",
            "cfop_interstate": "6102",
            "taxes": {
                "icms": {"origin": 0, "csosn": "102"},
                "pis": {"cst": "49"},
                "cofins": {"cst": "49"},
                "ibs_cbs": {
                    "cst": "000",
                    "c_class_trib": "000001",
                    "base": "4500.00",
                    "ibs_uf_rate": "0.10",
                    "ibs_uf_amount": "4.50",
                    "ibs_mun_rate": "0.00",
                    "ibs_mun_amount": "0.00",
                    "cbs_rate": "0.90",
                    "cbs_amount": "40.50",
                },
            },
        },
    )
    assert status == 201, (status, res)
    status, res = call("GET", f"/v1/companies/{company_id}/tax-profiles", token)
    assert status == 200 and len(res["data"]["items"]) == 1, res

    # 6. Register Company Webhook
    status, res = call(
        "POST",
        "/v1/webhooks",
        token,
        {
            "company_id": company_id,
            "url": "http://127.0.0.1:8088/healthz",
            "secret": "whsec-test-123456",
            "events": ["document.validated", "document.authorized", "document.rejected"],
        },
    )
    assert status == 201, (status, res)
    status, res = call("GET", "/v1/webhooks", token)
    assert status == 200 and len(res["data"]["items"]) >= 1, res

    # 7. Upload Synthetic Certificate & Test End-to-End Order-to-Invoice + Webhooks + Backup + MCP
    trust = ROOT / "var/secrets/icp_ca"
    if trust.read_bytes():
        raise RuntimeError("Refusing to replace an existing ICP trust store. Use a clean local test stack.")
    try:
        with tempfile.TemporaryDirectory() as folder:
            folder = pathlib.Path(folder)
            cmd(
                [
                    "openssl",
                    "req",
                    "-x509",
                    "-newkey",
                    "rsa:2048",
                    "-nodes",
                    "-keyout",
                    str(folder / "key.pem"),
                    "-out",
                    str(folder / "cert.pem"),
                    "-days",
                    "30",
                    "-subj",
                    "/C=BR/O=AllSpark Synthetic Test/CN=TESTE:11222333000181",
                    "-addext",
                    "subjectAltName=otherName:2.16.76.1.3.3;UTF8:11222333000181",
                ]
            )
            cmd(
                [
                    "openssl",
                    "pkcs12",
                    "-export",
                    "-out",
                    str(folder / "test.pfx"),
                    "-inkey",
                    str(folder / "key.pem"),
                    "-in",
                    str(folder / "cert.pem"),
                    "-passout",
                    "pass:synthetic-only",
                ]
            )
            trust.write_bytes((folder / "cert.pem").read_bytes())
            cert = {
                "pfx_base64": base64.b64encode((folder / "test.pfx").read_bytes()).decode(),
                "password": "synthetic-only",
            }
            status, res = call("POST", f"/v1/companies/{company_id}/certificates", token, cert)
            assert status == 201, (status, res)

            # Expiring Certificates Monitor
            status, res = call("GET", "/v1/certificates/expiring?days=60", token)
            assert status == 200 and len(res["data"]["items"]) >= 1, res

            # Beginner Order-to-Invoice with external_ref (`ref`), per-request `webhook_url`, and `tax_profile`
            ext_ref = "PEDIDO-" + uuid.uuid4().hex[:8].upper()
            status, res = call(
                "POST",
                "/v1/orders/invoice",
                token,
                {
                    "company_id": company_id,
                    "ref": ext_ref,
                    "operation": "generate",
                    "webhook_url": "http://127.0.0.1:8088/healthz",
                    "webhook_secret": "req-secret-999",
                    "tax_profile": "REVENDA_PADRAO",
                    "customer": {
                        "tax_id": "11144477735",
                        "name": "CLIENTE SIMPLIFICADO TESTE",
                        "ie_indicator": 9,
                        "address": {
                            "street": "Av Tancredo Neves",
                            "number": "500",
                            "district": "Caminho das Arvores",
                            "city_code": "2927408",
                            "city": "Salvador",
                            "state": "BA",
                            "postal_code": "41820021",
                        },
                    },
                    "items": [
                        {
                            "sku": "SKU-NOTEBOOK-01",
                            "description": "NOTEBOOK ULTRA PRO 16GB",
                            "quantity": "1.0000",
                            "price": "4500.00",
                        }
                    ],
                    "payment_method": "pix",
                },
                "order-key-" + uuid.uuid4().hex,
            )
            assert status == 202, (status, res)
            order_data = res["data"]
            doc_id = order_data["id"]
            assert order_data.get("ref") == ext_ref, order_data
            assert order_data.get("webhook_url") == "http://127.0.0.1:8088/healthz", order_data

            # Poll document by external ref (`/v1/documents/ref/{ref}`) until validated
            final_doc = None
            for _ in range(40):
                status, res = call("GET", f"/v1/documents/ref/{ext_ref}", token)
                assert status == 200, (status, res)
                d = res["data"]
                if d["status"] not in ("queued", "processing"):
                    final_doc = d
                    break
                time.sleep(0.3)

            assert final_doc is not None and final_doc["status"] == "validated", final_doc
            assert final_doc.get("ref") == ext_ref, final_doc

            # Trigger manual webhook resend & verify delivery
            status, res = call("POST", f"/v1/documents/ref/{ext_ref}/webhook/resend", token, {})
            assert status == 200 and len(res["data"].get("deliveries", [])) >= 1, (status, res)

            # Verify XML and PDF by external ref (`/v1/documents/ref/{ref}/xml` and `/pdf`)
            status, xml_bytes = call("GET", f"/v1/documents/ref/{ext_ref}/xml", token)
            assert status == 200 and b"<NFe" in xml_bytes and b"<IBSCBS>" in xml_bytes, "XML by ref missing NFe/IBSCBS"

            status, pdf_bytes = call("GET", f"/v1/documents/ref/{ext_ref}/pdf", token)
            assert status == 200 and pdf_bytes.startswith(b"%PDF"), "PDF by ref invalid"

            # Monthly Accountant ZIP Backup (`GET /v1/companies/{id}/backups/{YYYY-MM}.zip`)
            ym = time.strftime("%Y-%m")
            status, zip_bytes = call("GET", f"/v1/companies/{company_id}/backups/{ym}.zip", token)
            assert status == 200, f"backup zip failed with status {status}: {zip_bytes}"
            with zipfile.ZipFile(io.BytesIO(zip_bytes), "r") as zf:
                names = zf.namelist()
                assert f"{ym}/resumo-contador.json" in names, names
                assert any(n.endswith(".xml") for n in names), names
                assert any(n.endswith(".pdf") for n in names), names

            # Document types & Company Readiness (`/v1/document-types` & `/v1/companies/{id}/readiness`)
            status, dtypes = call("GET", "/v1/document-types", token)
            assert status == 200 and len(dtypes["data"]) >= 3 and len(dtypes["data"][0]["purposes"]) == 4, dtypes

            status, rdy = call("GET", f"/v1/companies/{company_id}/readiness", token)
            assert status == 200 and rdy["data"]["ready_for_nfe"] is True and rdy["data"]["has_valid_certificate"] is True, rdy

            # Order Preview (`/v1/orders/preview`)
            status, prev = call(
                "POST",
                "/v1/orders/preview",
                token,
                {
                    "company_id": company_id,
                    "tax_profile": "REVENDA_PADRAO",
                    "items": [{"sku": "SKU-PREV-1", "description": "ITEM SIMULADO", "quantity": "2.0000", "price": "100.00"}],
                },
            )
            assert status == 200 and prev["data"]["valid"] is True and prev["data"]["persisted"] is False, prev
            assert prev["data"]["resolved"]["icms_totals"]["vNF"] == "200.00", prev

            # MCP Server (`/v1/mcp` — Discovery, Resources, Prompts, 16+ Tools & Scope Enforcement)
            status, res = call("GET", "/v1/mcp", token)
            assert status == 200 and len(res["data"].get("tools", [])) >= 15, res
            assert res["data"]["security"]["scope_enforced"] is True, res

            status, mcp_rdy = call(
                "POST",
                "/v1/mcp",
                token,
                {
                    "jsonrpc": "2.0",
                    "id": 10,
                    "method": "tools/call",
                    "params": {"name": "get_company_readiness", "arguments": {"company_id": company_id}},
                },
            )
            assert status == 200 and mcp_rdy["result"]["isError"] is False, mcp_rdy
            rdy_payload = json.loads(mcp_rdy["result"]["content"][0]["text"])
            assert rdy_payload["ready_for_nfe"] is True, rdy_payload

            status, mcp_diag = call(
                "POST",
                "/v1/mcp",
                token,
                {
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "tools/call",
                    "params": {"name": "diagnose_fiscal_error", "arguments": {"code": "SEFAZ_656"}},
                },
            )
            assert status == 200, mcp_diag
            diag_payload = json.loads(mcp_diag["result"]["content"][0]["text"])
            assert diag_payload["sefaz_code"] == "656", diag_payload

            status, mcp_doc = call(
                "POST",
                "/v1/mcp",
                token,
                {
                    "jsonrpc": "2.0",
                    "id": 2,
                    "method": "tools/call",
                    "params": {"name": "get_document_status", "arguments": {"ref": ext_ref}},
                },
            )
            assert status == 200, mcp_doc
            mcp_doc_payload = json.loads(mcp_doc["result"]["content"][0]["text"])
            assert mcp_doc_payload["id"] == doc_id and mcp_doc_payload["status"] == "validated", mcp_doc_payload

            # Test STDIO Bridge (`scripts/mcp-stdio-bridge.py`)
            bridge_env = dict(os.environ, ALLSPARK_API_URL="http://127.0.0.1:8088", ALLSPARK_API_KEY=token)
            bridge_in = json.dumps({"jsonrpc": "2.0", "id": 99, "method": "initialize", "params": {}}) + "\n"
            bridge_out = subprocess.check_output(
                ["python3", str(ROOT / "scripts/mcp-stdio-bridge.py")],
                input=bridge_in,
                text=True,
                env=bridge_env,
            ).strip()
            bridge_json = json.loads(bridge_out)
            assert bridge_json["result"]["serverInfo"]["name"] == "allspark-fiscal-mcp", bridge_json

            # Test MCP Scope Enforcement with Read-Only Key
            sql(
                "UPDATE api_keys SET scopes=ARRAY['companies:read','certificates:read','documents:read'] "
                "WHERE tenant_id='" + t["tenant_id"] + "'"
            )
            # Read tool (`preview_order_invoice`) must succeed with read-only key
            status, mcp_ro_ok = call(
                "POST",
                "/v1/mcp",
                token,
                {
                    "jsonrpc": "2.0",
                    "id": 30,
                    "method": "tools/call",
                    "params": {
                        "name": "preview_order_invoice",
                        "arguments": {
                            "company_id": company_id,
                            "items": [{"description": "TESTE RO", "quantity": "1.0000", "price": "50.00"}],
                        },
                    },
                },
            )
            assert status == 200 and mcp_ro_ok["result"]["isError"] is False, mcp_ro_ok
            # Write tool (`emit_order_invoice`) must be blocked (`isError: True`, `FORBIDDEN`)
            status, mcp_ro_blocked = call(
                "POST",
                "/v1/mcp",
                token,
                {
                    "jsonrpc": "2.0",
                    "id": 31,
                    "method": "tools/call",
                    "params": {
                        "name": "emit_order_invoice",
                        "arguments": {
                            "company_id": company_id,
                            "items": [{"description": "TESTE BLOQUEADO", "quantity": "1.0000", "price": "50.00"}],
                        },
                    },
                },
            )
            assert status == 200 and mcp_ro_blocked["result"]["isError"] is True, mcp_ro_blocked
            blocked_payload = json.loads(mcp_ro_blocked["result"]["content"][0]["text"])
            assert blocked_payload["code"] == "FORBIDDEN", blocked_payload
    finally:
        trust.write_bytes(b"")

    print(
        "PASS: high-throughput worker pool, actionable errors, company readiness, order preview, tax profiles, "
        "order-to-invoice, external ref routes, per-request & company webhooks, logo DANFE, accountant ZIP backup, "
        "certificate expiration monitor, and hardened multi-AI MCP server + STDIO bridge."
    )


if __name__ == "__main__":
    main()
