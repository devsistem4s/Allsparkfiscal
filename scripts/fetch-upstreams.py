#!/usr/bin/env python3
"""Fetch reviewed upstream commits. Never reset or edit an existing checkout."""
import json,pathlib,subprocess
root=pathlib.Path(__file__).resolve().parent.parent
for repo in json.loads((root/'engines/upstream.lock.json').read_text()):
 target=root/'third_party'/repo['name']
 if target.exists():
  actual=subprocess.check_output(['git','-C',str(target),'rev-parse','HEAD'],text=True).strip()
  if actual!=repo['commit']:raise SystemExit(f"Preserved {target}: HEAD differs from lock; inspect manually.")
  print(repo['name']+': existing checkout preserved');continue
 target.parent.mkdir(exist_ok=True)
 subprocess.run(['git','init',str(target)],check=True,stdout=subprocess.DEVNULL)
 subprocess.run(['git','-C',str(target),'remote','add','origin',repo['url']],check=True)
 subprocess.run(['git','-C',str(target),'fetch','--depth','1','origin',repo['commit']],check=True)
 subprocess.run(['git','-C',str(target),'checkout','--detach','FETCH_HEAD'],check=True)
 print(repo['name']+': downloaded exact commit')
