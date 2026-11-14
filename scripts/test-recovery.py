#!/usr/bin/env python3
"""Failure injection only on a local development stack; never contacts SEFAZ."""
import json, time, uuid, subprocess
from integration import ROOT, tenant, call, cmd, sql

def main():
 if (ROOT/'var/secrets/icp_ca').read_bytes():raise RuntimeError('Run only on a development stack with an empty trust store.')
 worker_ids=cmd(['docker','compose','ps','-q','worker']).split()
 count=len(worker_ids)
 if not count:raise RuntimeError('Start workers before the recovery test.')
 t=tenant('recovery-test-'+uuid.uuid4().hex[:8])
 status,c=call('POST','/v1/companies',t['api_key'],json.loads((ROOT/'examples/company.json').read_text()));assert status==201,c
 doc=json.loads((ROOT/'examples/document.json').read_text());doc['company_id']=c['data']['id']
 cmd(['docker','compose','stop','worker'])
 try:
  ids=[]
  for phase in ['processing','transmitting']:
   status,a=call('POST','/v1/documents',t['api_key'],doc,'recovery-'+phase+'-'+uuid.uuid4().hex);assert status==202,a
   id=a['data']['id'];ids.append(id)
   sql("BEGIN; UPDATE jobs SET state='"+phase+"',lease_token='"+str(uuid.uuid4())+"',lease_until=now()-interval '1 second' WHERE document_id='"+id+"'; UPDATE documents SET status='"+phase+"' WHERE id='"+id+"'; UPDATE outbox SET published_at=now() WHERE job_id IN (SELECT id FROM jobs WHERE document_id='"+id+"'); COMMIT;")
  for _ in range(80):
   states=[call('GET','/v1/documents/'+id,t['api_key'])[1]['data']['status'] for id in ids]
   if states==['queued','reconciliation_required']:break
   time.sleep(.25)
  assert states==['queued','reconciliation_required'],states
  assert sql("SELECT count(*) FROM jobs WHERE document_id='"+ids[1]+"' AND state='done'")=='1'
  report={'passed':True,'checks':['expired pre-transmission lease is requeued','expired transmission lease requires reconciliation','uncertain transmission is never automatically resubmitted'],'sefaz_requests':0}
  (ROOT/'var/reports/recovery.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
 finally:cmd(['docker','compose','up','-d','--no-deps','--scale','worker='+str(count),'worker'])
if __name__=='__main__':main()
