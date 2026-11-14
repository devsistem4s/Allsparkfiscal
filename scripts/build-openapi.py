#!/usr/bin/env python3
"""Generate separate executable and planned API contracts; no planned success responses."""
import json,pathlib
root=pathlib.Path(__file__).resolve().parent.parent
S={}
def obj(properties,required=()):return {'type':'object','additionalProperties':False,'properties':properties,'required':list(required)}
def string(**kw):return {'type':'string',**kw}
def ref(name):return {'$ref':'#/components/schemas/'+name}
uuid=string(format='uuid');decimal=string(pattern=r'^[0-9]{1,12}(\.[0-9]{1,10})?$')
S['APIError']=obj({'code':string(),'message':string(),'source':string(),'retryable':{'type':'boolean'}},['code','message','source','retryable'])
S['Envelope']=obj({'data':{},'error':{'oneOf':[ref('APIError'),{'type':'null'}]},'request_id':uuid},['data','error','request_id'])
S['Company']=obj({'id':uuid,'cnpj':string(pattern='^[A-Z0-9]{12}[0-9]{2}$'),'name':string(minLength=2,maxLength=120),'uf':string(minLength=2,maxLength=2),'ie':string(minLength=2,maxLength=20),'crt':{'type':'integer','minimum':1,'maximum':4}},['cnpj','name','uf','ie','crt'])
S['CertificateUpload']=obj({'pfx_base64':string(contentEncoding='base64',maxLength=2097152),'password':string(maxLength=1024),'csc':string(),'csc_id':string()},['pfx_base64','password'])
S['Address']=obj({k:string() for k in ['street','number','district','city_code','city','state','postal_code']},['street','number','district','city_code','city','state','postal_code'])
S['Recipient']=obj({'tax_id':string(),'name':string(),'ie_indicator':{'type':'integer'},'ie':string(),'address':ref('Address')},['tax_id','name','ie_indicator','address'])
S['Tax']=obj({'cst':string(),'csosn':string(),'origin':{'type':'integer'},'base':decimal,'rate':decimal,'amount':decimal})
S['Taxes']=obj({'icms':ref('Tax'),'pis':ref('Tax'),'cofins':ref('Tax')},['icms','pis','cofins'])
S['Item']=obj({**{k:string() for k in ['sku','description','ncm','cfop','gtin','unit']},'quantity':decimal,'unit_price':decimal,'total':decimal,'taxes':ref('Taxes')},['sku','description','ncm','cfop','unit','quantity','unit_price','total','taxes'])
S['Payment']=obj({'method':string(),'amount':decimal},['method','amount'])
S['Invoice']=obj({'nature':string(),'direction':{'type':'integer','enum':[0,1]},'destination':{'type':'integer','enum':[1,2,3]},'city_code':string(),'final_consumer':{'type':'boolean'},'presence':{'type':'integer'},'issuer_address':ref('Address'),'recipient':ref('Recipient'),'items':{'type':'array','minItems':1,'maxItems':990,'items':ref('Item')},'payments':{'type':'array','minItems':1,'maxItems':100,'items':ref('Payment')},'freight_mode':{'type':'integer'},'additional_info':string()},['nature','direction','destination','city_code','final_consumer','presence','issuer_address','recipient','items','payments','freight_mode'])
S['DocumentInput']=obj({'company_id':uuid,'kind':string(enum=['nfe','nfce']),'operation':string(enum=['generate','issue']),'environment':string(enum=['homologation']),'series':{'type':'integer','minimum':0,'maximum':889},'invoice':ref('Invoice')},['company_id','kind','operation','environment','series','invoice'])
S['BulkInput']=obj({'documents':{'type':'array','minItems':1,'maxItems':50,'items':obj({'idempotency_key':string(minLength=8,maxLength=128),'document':ref('DocumentInput')},['idempotency_key','document'])}},['documents'])
S['Artifact']=obj({'id':uuid,'kind':string(),'media_type':string(),'sha256':string(),'size':{'type':'integer'},'download_url':string()},['id','kind','media_type','sha256','size','download_url'])
S['Document']=obj({'id':uuid,'company_id':uuid,'kind':string(),'operation':string(),'environment':string(),'series':{'type':'integer'},'number':{'type':'integer','description':'Synthetic number for generate; reserved fiscal sequence only for issue.'},'status':string(enum=['queued','processing','validated','transmitting','authorized','rejected','reconciliation_required','failed']),'access_key':{'type':['string','null']},'result':{'type':'object'},'created_at':string(format='date-time'),'artifacts':{'type':'array','items':ref('Artifact')}},['id','company_id','kind','status','result','artifacts'])
def response(desc):return {'description':desc,'content':{'application/json':{'schema':ref('Envelope')}}}
def op(summary,status,body=None,auth=True,desc=''):
 data={'summary':summary,'description':desc,'security':[{'bearerAuth':[]}] if auth else [],'responses':{str(status):response('Resultado da operação')}}
 if auth:
  for k,txt in {401:'Credencial inválida',403:'Scope insuficiente',404:'Recurso ausente ou de outro tenant',409:'Conflito',422:'Dados inválidos ou não suportados',429:'Limite de requisições/backpressure',503:'Dependência indisponível'}.items():data['responses'][str(k)]=response(txt)
 if body:data['requestBody']={'required':True,'content':{'application/json':{'schema':ref(body)}}}
 return data
paths={
 '/healthz':{'get':op('Saúde do processo',200,auth=False)},
 '/readyz':{'get':op('Prontidão PostgreSQL/Redis',200,auth=False)},
 '/v1/capabilities':{'get':op('Capacidades da base',200)},
 '/v1/companies':{'post':op('Cadastrar empresa',201,'Company')},
 '/v1/companies/{id}/certificates':{'post':op('Cadastrar/substituir certificado A1',201,'CertificateUpload',desc='Requer cadeia confiável provisionada. Nunca retorna PFX ou senha.')},
 '/v1/documents':{'post':op('Aceitar geração ou emissão assíncrona',202,'DocumentInput',desc='202 significa persistência. Prévia não reserva número. Emissão desabilitada por padrão; somente homologação.')},
 '/v1/documents/bulk':{'post':op('Aceitar até 50 documentos com resultado individual',207,'BulkInput')},
 '/v1/documents/{id}':{'get':op('Consultar documento e arquivos',200)},
 '/v1/documents/{id}/reconcile':{'post':op('Consultar resultado fiscal desconhecido; nunca reenviar',202)},
 '/v1/documents/{id}/artifacts/{artifactId}':{'get':op('Baixar artefato autorizado para o tenant',200)}
}
S['Company']['properties'].pop('id')
S['CompanyPatch']=obj({k:S['Company']['properties'][k] for k in ['name','ie','crt']})
S['FiscalSettingsInput']=obj({'environment':string(enum=['homologation']),'series':{'type':'integer','minimum':0,'maximum':889},'enabled':{'type':'boolean'}},['environment','series','enabled'])
paths['/v1/companies']['get']=op('Listar empresas do tenant',200)
paths['/v1/companies/{id}']={m:op(title,200,body) for m,title,body in [('get','Consultar empresa',None),('patch','Editar razão social, IE e CRT','CompanyPatch'),('delete','Desativar empresa preservando histórico',None)]}
paths['/v1/companies/{id}/certificate']={m:op(title,201 if m=='put' else 200,body) for m,title,body in [('get','Consultar metadados do certificado ativo',None),('put','Cadastrar ou substituir certificado','CertificateUpload'),('delete','Desativar certificado',None)]}
paths['/v1/companies/{id}/fiscal']={'get':op('Consultar configurações fiscais',200)}
for kind in ['nfe','nfce']:paths['/v1/companies/{id}/fiscal/'+kind]={'put':op('Configurar tipo, série e ambiente','200','FiscalSettingsInput')}
paths['/v1/document-types']={'get':op('Catálogo de documentos implementados e futuros',200)}
paths['/v1/documents/validate']={'post':op('Validar entrada sem persistir, assinar ou autorizar',200,'DocumentInput',desc='Não valida XSD, certificado, cadastro SEFAZ ou habilitação fiscal. Não reserva número nem publica na fila.')}
paths['/v1/documents']['get']=op('Listar documentos do tenant',200)
for path in ['/v1/companies','/v1/documents']:
 paths[path]['get']['parameters']=[{'name':'cursor','in':'query','schema':uuid},{'name':'limit','in':'query','schema':{'type':'integer','minimum':1,'maximum':100,'default':50}}]
paths['/v1/documents']['get']['parameters'] += [{'name':'company_id','in':'query','schema':uuid},{'name':'kind','in':'query','schema':string(enum=['nfe','nfce'])}]
S['Company']['properties']['address']=ref('Address')
S['Company']['required'].append('address')
S['CompanyPatch']['properties']['address']=ref('Address')
S['Invoice']['required'].remove('issuer_address')
S['Invoice']['properties']['issuer_address']['description']='Legado: ignorado pelo worker; o endereço do emitente vem do cadastro da empresa.'
paths['/readyz']['get']['responses']['503']=response('Dependência indisponível')
paths['/v1/documents']['post']['parameters']=[{'name':'Idempotency-Key','in':'header','required':True,'schema':string(minLength=8,maxLength=128)}]
for path,item in paths.items():
 for name in ['id','artifactId']:
  if '{'+name+'}' in path:item.setdefault('parameters',[]).append({'name':name,'in':'path','required':True,'schema':uuid})
paths['/v1/documents/{id}/artifacts/{artifactId}']['get']['responses']['200']={'description':'Conteúdo binário privado; hash e MIME na consulta de documento','content':{'application/xml':{'schema':{'type':'string','format':'binary'}},'application/pdf':{'schema':{'type':'string','format':'binary'}}}}
spec={'openapi':'3.1.0','info':{'title':'AllSpark Fiscal — API executável','version':'0.2.0','description':'Fundação de desenvolvimento. Geração local testada; transmissão não homologada e desabilitada por padrão. Ver docs/IMPLEMENTATION_STATUS.md.'},'servers':[{'url':'http://127.0.0.1:8088'}],'paths':paths,'components':{'securitySchemes':{'bearerAuth':{'type':'http','scheme':'bearer','description':'API key de tenant no bootstrap atual; não é OAuth2.'}},'schemas':S}}
(root/'api/openapi.json').write_text(json.dumps(spec,indent=2,ensure_ascii=False)+'\n')
(root/'openapi').mkdir(exist_ok=True)
# JSON is valid YAML 1.2; use a conventional .yaml alias without a YAML dependency.
(root/'openapi/openapi.yaml').write_text(json.dumps(spec,indent=2,ensure_ascii=False)+'\n')
planned={}
def add(path,methods):
 for method in methods.split(','):
  entry=planned.setdefault(path,{})
  entry[method]={'summary':'Planejado — não implementado','x-implementation-status':'planned','responses':{'501':{'description':'Especificação de destino; não há handler implementado nesta versão.'}}}
  for part in path.split('/'):
   if part.startswith('{'):entry['parameters']=[{'name':x[1:-1],'in':'path','required':True,'schema':uuid} for x in path.split('/') if x.startswith('{')]
for path in ['/v1/auth/token','/v1/auth/refresh','/v1/auth/revoke']:add(path,'post')
add('/v1/companies','get');add('/v1/companies/{id}','get,patch,delete');add('/v1/companies/{id}/certificate','get,put,delete');add('/v1/companies/{id}/fiscal','get')
for kind in ['nfe','nfce','nfse']:
 add('/v1/companies/{id}/fiscal/'+kind,'put');add('/v1/'+kind,'post,get');add('/v1/'+kind+'/{id}','get');add('/v1/'+kind+'/validate','post')
 for end in ['xml','pdf','files','events']:add('/v1/'+kind+'/{id}/'+end,'get')
 add('/v1/'+kind+'/{id}/cancel','post')
add('/v1/nfe/{id}/correction-letter','post');add('/v1/nfe/void-number-range','post');add('/v1/nfse/{id}/replace','post');add('/v1/documents','get');add('/v1/documents/{id}/events','get')
for group in ['batches','imports']:
 add('/v1/'+group,'post');add('/v1/'+group+'/{id}','get')
add('/v1/batches/{id}/documents','get');add('/v1/webhooks','post,get');add('/v1/webhooks/{id}','get,patch,delete');add('/v1/webhooks/{id}/deliveries','get')
for path in list(planned):
 for method in list(planned[path]):
  if method in paths.get(path,{}):planned[path].pop(method)
 if not any(m in planned[path] for m in ['get','post','put','patch','delete']):planned.pop(path)
future={'openapi':'3.1.0','info':{'title':'AllSpark Fiscal — proposta futura NÃO EXECUTÁVEL','version':'0.0.0-planned','description':'Não usar como client SDK atual. 501 é marcador de planejamento; rotas inexistentes no runtime retornam 404.'},'paths':planned}
(root/'openapi/planned.json').write_text(json.dumps(future,indent=2,ensure_ascii=False)+'\n')
print('Generated executable API contract and separate planned surface.')
