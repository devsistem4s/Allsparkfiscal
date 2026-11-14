#!/usr/bin/env python3
"""Create local secrets exclusively; never overwrite keys or an existing .env."""
import base64, os, secrets
from pathlib import Path
root=Path(__file__).resolve().parent.parent
os.umask(0o077)
for name in ['secrets','objects','reports']:
 (root/'var'/name).mkdir(parents=True,exist_ok=True)
for name in ['db_password','db_admin_password','rabbit_password','redis_password','engine_token','master_key']:
 p=root/'var/secrets'/name
 if not p.exists():
  with p.open('x') as f:f.write((base64.b64encode(secrets.token_bytes(32)).decode() if name=='master_key' else secrets.token_hex(32))+'\n')
# Fail-closed trust store: replace with a reviewed ICP-Brasil root CA bundle.
p=root/'var/secrets/icp_ca'
if not p.exists():p.touch(mode=0o600)
p=root/'.env'
if not p.exists():
 with p.open('x') as f:f.write(f'LOCAL_UID={os.getuid()}\nLOCAL_GID={os.getgid()}\nAPI_PORT=8088\nENABLE_ISSUANCE=false\nTENANT_RATE_PER_SECOND=200\n')
print('Local secrets ready; existing files preserved. ICP trust store requires configuration.')
