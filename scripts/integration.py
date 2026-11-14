#!/usr/bin/env python3
"""Local integration test. Synthetic certificate only, no external fiscal requests."""
import base64, concurrent.futures, hashlib, json, os, pathlib, subprocess, tempfile, time, urllib.request, urllib.error, uuid
ROOT=pathlib.Path(__file__).resolve().parent.parent
os.chdir(ROOT)
BASE=os.environ.get('API_URL','http://127.0.0.1:8088')
def cmd(args,**kwargs):
 return subprocess.check_output(args,text=True,stderr=subprocess.PIPE,**kwargs)
def tenant(name):
 raw=cmd(['docker','compose','run','--rm','--no-deps','-e','ROLE=tenant-create','-e','TENANT_NAME='+name,'api'])
 return json.loads(raw)
def call(method,path,token=None,body=None,key=None):
 data=json.dumps(body).encode() if body is not None else None
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 if key:headers['Idempotency-Key']=key
 req=urllib.request.Request(BASE+path,data=data,headers=headers,method=method)
 try:res=urllib.request.urlopen(req,timeout=110)
 except urllib.error.HTTPError as e:res=e
 raw=res.read()
 return res.status,json.loads(raw) if res.headers.get('Content-Type','').startswith('application/json') else raw

def sql(query):
 return cmd(['docker','compose','exec','-T','postgres','psql','-U','allspark_owner','-d','allspark','-At','-c',query]).strip()
def main():
 # Inspect the running services, rather than an .env that may differ from Docker.
 probe=tenant('integration-guard-'+uuid.uuid4().hex[:8])
 caps=call('GET','/v1/capabilities',probe['api_key'])
 if caps[0]!=200 or caps[1]['data']['issuance_enabled']:raise RuntimeError('Integration requires runtime issuance disabled.')
 guard=cmd(['docker','compose','exec','-T','engine','php','-r',"echo getenv('ENABLE_ISSUANCE') === 'true' ? 'enabled' : 'disabled';"])
 if guard!='disabled':raise RuntimeError('Engine transmission must be disabled.')
 started=time.monotonic()
 assert call('GET','/healthz')[0]==200
 assert call('GET','/readyz')[0]==200
 assert call('GET','/v1/capabilities')[0]==401
 a=tenant('integration-a-'+uuid.uuid4().hex[:8]);b=tenant('integration-b-'+uuid.uuid4().hex[:8])
 ca=call('POST','/v1/companies',a['api_key'],json.loads((ROOT/'examples/company.json').read_text()))
 assert ca[0]==201,ca
 company=ca[1]['data']['id']
 doc=json.loads((ROOT/'examples/document.json').read_text());doc['company_id']=company
 doc['invoice']['issuer_address']={'street':'UNTRUSTED OVERRIDE','state':'SP'}
 assert call('POST','/v1/documents',b['api_key'],doc,'cross-tenant-123')[0]==404
 assert call('POST',f'/v1/companies/{company}/certificates',b['api_key'],{})[0]==404
 production=dict(doc,environment='production')
 assert call('POST','/v1/documents',a['api_key'],production,'production-123')[0]==422
 assert call('POST','/v1/documents',a['api_key'],dict(doc,operation='issue'),'issue-123')[0]==422
 key='concurrency-'+uuid.uuid4().hex
 with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
  responses=list(pool.map(lambda _:call('POST','/v1/documents',a['api_key'],doc,key),range(16)))
 assert all(x[0]==202 for x in responses),responses
 ids={x[1]['data']['id'] for x in responses};assert len(ids)==1,ids
 document=next(iter(ids))
 assert sql("SELECT count(*) FROM jobs WHERE document_id='"+document+"'")=='1'
 assert call('GET','/v1/documents/'+document,b['api_key'])[0]==404
 assert call('POST','/v1/documents',a['api_key'],dict(doc,series=2),key)[0]==409
 assert sql("SELECT count(*) FROM numbering WHERE tenant_id='"+a['tenant_id']+"'")=='0'
 # Allow the first no-certificate job to settle in durable failure/DLQ.
 for _ in range(40):
  result=call('GET','/v1/documents/'+document,a['api_key'])[1]['data']
  if result['status']=='failed':break
  time.sleep(.25)
 assert result['status']=='failed',result
 assert result['result']['errors'][0]['code']=='CERTIFICATE_REQUIRED',result
 batch={'documents':[{'idempotency_key':'bulk-valid-'+uuid.uuid4().hex,'document':doc},{'idempotency_key':'bulk-invalid-'+uuid.uuid4().hex,'document':dict(doc,kind='nfse')}]}
 status,batch_result=call('POST','/v1/documents/bulk',a['api_key'],batch)
 assert status==207 and [item['status'] for item in batch_result['data']]==[202,422],batch_result
 # Certificate replacement intentionally waits for all active work to finish.
 bulk_id=batch_result['data'][0]['id']
 for _ in range(120):
  state=call('GET','/v1/documents/'+bulk_id,a['api_key'])[1]['data']['status']
  if state=='failed':break
  time.sleep(.25)
 assert state=='failed',state
 trust=ROOT/'var/secrets/icp_ca'
 if trust.read_bytes():raise RuntimeError('Refusing to replace an existing ICP trust store. Use a clean local test stack.')
 try:
  with tempfile.TemporaryDirectory() as folder:
   folder=pathlib.Path(folder)
   cmd(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-keyout',str(folder/'key.pem'),'-out',str(folder/'cert.pem'),'-days','1','-subj','/C=BR/O=AllSpark Synthetic Test/CN=TESTE:11222333000181','-addext','subjectAltName=otherName:2.16.76.1.3.3;UTF8:11222333000181'])
   cmd(['openssl','pkcs12','-export','-out',str(folder/'test.pfx'),'-inkey',str(folder/'key.pem'),'-in',str(folder/'cert.pem'),'-passout','pass:synthetic-only'])
   cert={'pfx_base64':base64.b64encode((folder/'test.pfx').read_bytes()).decode(),'password':'synthetic-only'}
   assert call('POST',f'/v1/companies/{company}/certificates',a['api_key'],cert)[0]==422
   # Only a synthetic CA is trusted during this local test; restore even on failure.
   trust.write_bytes((folder/'cert.pem').read_bytes())
   uploaded=call('POST',f'/v1/companies/{company}/certificates',a['api_key'],cert)
   assert uploaded[0]==201,uploaded
   stored=sql("SELECT encode(sealed,'hex') FROM certificates WHERE tenant_id='"+a['tenant_id']+"' AND active")
   cipher=bytes.fromhex(stored)
   assert b'synthetic-only' not in cipher and (folder/'test.pfx').read_bytes() not in cipher
   assert cipher[0]==2,'envelope encryption v2 expected'
   accepted=call('POST','/v1/documents',a['api_key'],doc,'generation-'+uuid.uuid4().hex)
   assert accepted[0]==202,accepted
   generated=accepted[1]['data']['id']
   for _ in range(120):
    result=call('GET','/v1/documents/'+generated,a['api_key'])[1]['data']
    if result['status'] not in ['queued','processing']:break
    time.sleep(.5)
   assert result['status']=='validated',result
   artifacts={x['kind']:x for x in result['artifacts']}
   assert 'signed_xml' in artifacts and 'preview_pdf' in artifacts,result
   for kind,item in artifacts.items():
    status,raw=call('GET',item['download_url'],a['api_key']);assert status==200
    assert hashlib.sha256(raw).hexdigest()==item['sha256']
    if kind=='preview_pdf':assert raw.startswith(b'%PDF-')
    if kind=='signed_xml':assert b'UNTRUSTED OVERRIDE' not in raw and b'RUA TESTE' in raw
    assert call('GET',item['download_url'],b['api_key'])[0]==404
    encrypted=(ROOT/'var/objects'/item['id']).read_bytes()
    assert raw not in encrypted and encrypted[0]==2
   # Fixture statuses exercise certificate renewal without issuing anything.
   try:
    sql("UPDATE documents SET status='processing' WHERE id='"+generated+"'")
    assert call('PUT',f'/v1/companies/{company}/certificate',a['api_key'],cert)[0]==409
    sql("UPDATE documents SET status='reconciliation_required' WHERE id='"+generated+"'")
    rotated=call('PUT',f'/v1/companies/{company}/certificate',a['api_key'],cert)
    assert rotated[0]==201,rotated
    assert call('PATCH',f'/v1/companies/{company}',a['api_key'],{'name':'Blocked change'})[0]==409
   finally:sql("UPDATE documents SET status='validated' WHERE id='"+generated+"'")
   report={'passed':True,'elapsed_seconds':round(time.monotonic()-started,2),'checks':['health/readiness','authentication','tenant isolation: company/certificate/document/artifact','production and transmission guards','16 concurrent idempotent submits create one job','idempotency conflict','preview does not consume numbering','missing certificate becomes permanent failure','untrusted certificate denied','encrypted certificate with envelope v2','RabbitMQ worker processing','bulk returns independent 202/422 results','signed XML + XSD + PDF','encrypted downloads + SHA256','certificate renewal allowed for reconciliation, blocked during active work'],'sefaz_requests':0}
   (ROOT/'var/reports/integration.json').write_text(json.dumps(report,indent=2)+'\n')
   print(json.dumps(report,indent=2))
 finally:trust.write_bytes(b'')
if __name__=='__main__':main()
