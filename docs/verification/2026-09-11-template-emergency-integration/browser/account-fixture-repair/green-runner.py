from pathlib import Path
import subprocess, os, json, hashlib, time, re
root=Path('/private/tmp/rcc-template-notification-integration')
evidence=root/'docs/verification/2026-09-11-template-emergency-integration/browser'
evidence.mkdir(parents=True,exist_ok=True)
selections=[('07-release-approvals','^TestAccountBrowserSystemPath$/^release-approvals\\.cjs$')]
def snapshot():
    names=set(subprocess.check_output(['git','ls-files','-z'],cwd=root).decode().split('\0'))
    return {p:hashlib.sha256((root/p).read_bytes()).hexdigest() for p in sorted(names) if p and (root/p).is_file() and (p.startswith('admin/') or p.startswith('web/')) and not p.startswith('web/node_modules/')}
all_results=[]
for label,selector in selections:
    output=evidence/label
    output.mkdir(exist_ok=False)
    before=snapshot()
    (output/'source-input.json').write_text(json.dumps(before,indent=2)+'\n')
    cmd=['go','test','-tags=integration,browser','./cmd/admin','-run',selector,'-count=1','-v','-timeout=8m']
    env={**os.environ,'TESTCONTAINERS_RYUK_DISABLED':'true','DOCKER_HOST':'unix:///Users/asher/.colima/default/docker.sock','GOCACHE':'/private/tmp/rcc-go-cache','RCC_E2E_OUTPUT':str(output)}
    started=time.time()
    with (evidence/(label+'.log')).open('wb') as log:
        result=subprocess.run(cmd,cwd=root/'admin',env=env,stdout=log,stderr=subprocess.STDOUT)
    after=snapshot()
    raw=(evidence/(label+'.log')).read_text()
    created=sorted(set(re.findall(r'Container created: ([0-9a-f]+)',raw)))
    terminated=sorted(set(re.findall(r'Container terminated: ([0-9a-f]+)',raw)))
    container_checks=[]
    for identifier in created:
        inspected=subprocess.run(['docker','inspect','--format','{{.Id}}',identifier],env=env,capture_output=True,text=True)
        container_checks.append({'id':identifier,'inspect_exit':inspected.returncode,'stdout':inspected.stdout,'stderr':inspected.stderr})
    record={'command':cmd,'cwd':str(root/'admin'),'output':str(output),'started_unix':started,'duration_seconds':time.time()-started,'exit_code':result.returncode,'source_changed':[p for p in sorted(set(before)|set(after)) if before.get(p)!=after.get(p)],'created_containers':created,'terminated_containers':terminated,'cleanup_inspection':container_checks,'browser':'chromium / installed Chrome channel (default)' }
    (evidence/(label+'.json')).write_text(json.dumps(record,indent=2)+'\n')
    all_results.append(record)
    print(label,'exit',result.returncode,'seconds',round(record['duration_seconds'],2),'source_changes',len(record['source_changed']),flush=True)
(evidence/'07-release-approvals-results.json').write_text(json.dumps(all_results,indent=2)+'\n')
raise SystemExit(1 if any(r['exit_code'] for r in all_results) else 0)
