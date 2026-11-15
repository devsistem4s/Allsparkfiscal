#!/usr/bin/env python3
"""
AllSpark Fiscal — Gerador de Novo Tenant e API Key para Clientes
Uso:
  python3 create-client-token.py "Nome do Cliente ou Software House" [--vps]
"""
import sys, secrets, base64, hashlib, uuid, subprocess

def create_tenant(name: str, execute_vps: bool = False):
    token_bytes = secrets.token_bytes(32)
    api_key = "af_" + base64.urlsafe_b64encode(token_bytes).decode().rstrip("=")
    digest_hex = hashlib.sha256(api_key.encode()).hexdigest()
    tenant_id = str(uuid.uuid4())
    key_id = str(uuid.uuid4())
    
    sql = f"""
    INSERT INTO tenants (id, name) VALUES ('{tenant_id}', '{name}');
    INSERT INTO api_keys (id, tenant_id, digest, scopes) 
    VALUES ('{key_id}', '{tenant_id}', decode('{digest_hex}', 'hex'), '{{\"companies:read\", \"companies:write\", \"certificates:read\", \"certificates:write\", \"documents:write\", \"documents:read\"}}');
    """
    
    if execute_vps:
        cmd = f"""ssh vpsallspark "docker exec \\$(docker ps -qf name=allsparkfiscal_postgres) psql -U allspark allspark -c \\"{sql}\\"" """
        res = subprocess.run(cmd, shell=True, capture_output=True, text=True)
        if res.returncode != 0:
            print(f"Erro ao criar na VPS: {res.stderr}")
            sys.exit(1)
        print("✅ Tenant provisionado com sucesso na VPS de Produção!")
    
    print("=" * 60)
    print(f"🏢 Cliente/Tenant:  {name}")
    print(f"🆔 Tenant ID:       {tenant_id}")
    print(f"🔑 API Key (Token): {api_key}")
    print("=" * 60)
    print("Guarde o Token em local seguro. Por segurança, ele é armazenado")
    print("apenas em formato de hash SHA-256 no banco de dados.")
    return tenant_id, api_key

if __name__ == "__main__":
    client_name = sys.argv[1] if len(sys.argv) > 1 and not sys.argv[1].startswith("--") else "Novo Cliente SaaS"
    use_vps = "--vps" in sys.argv
    create_tenant(client_name, execute_vps=use_vps)
