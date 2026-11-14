#!/usr/bin/env python3
"""Measure local durable HTTP admission only; no certificate and no SEFAZ calls."""
import argparse, concurrent.futures, json, time, uuid
from integration import ROOT, tenant, call, sql

def main():
 p=argparse.ArgumentParser();p.add_argument('--requests',type=int,default=1000);p.add_argument('--rate',type=float,default=50);p.add_argument('--concurrency',type=int,default=20);args=p.parse_args()
 if not 1<=args.requests<=100000 or not 0<args.rate<=1000 or not 1<=args.concurrency<=200:p.error('requests 1..100000; rate 0..1000; concurrency 1..200')
 t=tenant('load-test-'+uuid.uuid4().hex[:8])
 status,response=call('POST','/v1/companies',t['api_key'],json.loads((ROOT/'examples/company.json').read_text()))
 assert status==201,response
 doc=json.loads((ROOT/'examples/document.json').read_text());doc['company_id']=response['data']['id']
 prefix=uuid.uuid4().hex
 def run(i):
  start=time.perf_counter()
  try:status,result=call('POST','/v1/documents',t['api_key'],doc,f'load-{prefix}-{i}')
  except Exception:return 0,time.perf_counter()-start
  return status,time.perf_counter()-start
 start=time.perf_counter();futures=[]
 with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as pool:
  for i in range(args.requests):
   remaining=start+i/args.rate-time.perf_counter()
   if remaining>0:time.sleep(remaining)
   futures.append(pool.submit(run,i))
  outcomes=[f.result() for f in futures]
 elapsed=time.perf_counter()-start;durations=sorted(d for _,d in outcomes);codes={str(s):sum(1 for c,_ in outcomes if c==s) for s in set(s for s,_ in outcomes)}
 persisted=int(sql("SELECT count(*) FROM documents WHERE tenant_id='"+t['tenant_id']+"'"))
 report={'scenario':'durable HTTP admission; missing certificates; no fiscal issuance','requests':args.requests,'target_rate_per_second':args.rate,'concurrency':args.concurrency,'elapsed_seconds':round(elapsed,3),'achieved_requests_per_second':round(args.requests/elapsed,2),'http_status_counts':codes,'persisted_documents':persisted,'latency_ms':{'p50':round(durations[int((len(durations)-1)*.50)]*1000,2),'p95':round(durations[int((len(durations)-1)*.95)]*1000,2),'p99':round(durations[int((len(durations)-1)*.99)]*1000,2)},'sefaz_requests':0}
 (ROOT/'var/reports/load-test.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
 if codes!={'202':args.requests} or persisted!=args.requests:raise SystemExit('Admission test did not accept all documents; inspect the report.')
if __name__=='__main__':main()
