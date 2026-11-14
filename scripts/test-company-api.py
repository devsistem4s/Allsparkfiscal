#!/usr/bin/env python3
"""Company lifecycle/preflight: no certificate upload or SEFAZ transmission."""
from integration import tenant,call,sql,ROOT,cmd
import json,uuid

def main():
 a=tenant('company-api-'+uuid.uuid4().hex[:8]);b=tenant('company-isolation-'+uuid.uuid4().hex[:8]);token=a['api_key']
 def request(method,path,body=None,status=200):
  result=call(method,path,token,body);assert result[0]==status,(method,path,result);return result[1]['data']
 c=request('POST','/v1/companies',json.loads((ROOT/'examples/company.json').read_text()),201);cid=c['id'];base='/v1/companies/'+cid
 assert request('GET',base)['archived_at'] is None
 assert request('GET',base)['address']['state']=='BA'
 bad=dict(c['address'],state='SP');request('PATCH',base,{'address':bad},422)
 assert call('POST','/v1/auth/register',token,{})[0]==404
 assert call('POST','/v1/users',token,{})[0]==404
 missing=json.loads((ROOT/'examples/company.json').read_text());missing.pop('address')
 request('POST','/v1/companies',missing,422)
 assert call('GET',base,b['api_key'])[0]==404
 assert call('PATCH',base,b['api_key'],{'name':'Intruso'})[0]==404
 request('PATCH',base,{'name':'Empresa Atualizada'})
 request('PATCH',base,{'cnpj':'11222333000181'},422)
 assert request('GET','/v1/companies?limit=1')['items'][0]['id']==cid
 request('GET','/v1/companies?limit=101',status=422)
 assert request('GET',base+'/fiscal')==[]
 doc=json.loads((ROOT/'examples/document.json').read_text());doc['company_id']=cid
 assert request('POST','/v1/documents/validate',doc)['persisted'] is False
 assert call('POST','/v1/documents/validate',b['api_key'],doc)[0]==404
 issue=dict(doc,operation='issue')
 code,res=call('POST','/v1/documents',token,issue,'preflight-'+uuid.uuid4().hex);assert code==422 and res['error']['code']=='FISCAL_CONFIG_REQUIRED',res
 request('PUT',base+'/fiscal/nfe',{'environment':'homologation','series':doc['series'],'enabled':True})
 code,res=call('POST','/v1/documents',token,issue,'preflight-'+uuid.uuid4().hex);assert code==422 and res['error']['code']=='CERTIFICATE_REQUIRED',res
 assert sql("SELECT count(*) FROM documents WHERE tenant_id='"+a['tenant_id']+"'")=='0'
 assert sql("SELECT count(*) FROM numbering WHERE tenant_id='"+a['tenant_id']+"'")=='0'
 request('GET',base+'/certificate',status=404)
 request('DELETE',base+'/certificate')
 request('DELETE',base)
 request('POST','/v1/documents/validate',doc,409)
 request('PATCH',base,{'name':'Desativada'},409)
 assert request('GET',base)['archived_at']
 assert request('GET','/v1/documents')['items']==[]
 # Read-only keys cannot mutate any of the new resources.
 sql("UPDATE api_keys SET scopes=ARRAY['companies:read','certificates:read','documents:read'] WHERE tenant_id='"+a['tenant_id']+"'")
 request('GET',base)
 request('DELETE',base,status=403)
 request('PUT',base+'/fiscal/nfe',{'environment':'homologation','series':1,'enabled':True},403)
 request('DELETE',base+'/certificate',status=403)
 key_id=sql("SELECT id FROM api_keys WHERE tenant_id='"+a['tenant_id']+"'")
 cmd(['docker','compose','run','--rm','--no-deps','-e','ROLE=key-revoke','-e','KEY_ID='+key_id,'api'])
 assert call('GET',base,token)[0]==401
 print('PASS: cadastro, edição, paginação, isolamento, scopes, configuração fiscal, validação sem efeitos, preflight sem numeração, desativação. Nenhuma transmissão fiscal.')
if __name__=='__main__':main()
