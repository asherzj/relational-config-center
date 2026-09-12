"""Actual version supervisor filesystem checks; package installation/browser run are skipped."""
import hashlib, json, os, pathlib, shutil, subprocess, tempfile
root = pathlib.Path(__file__).resolve().parents[4]
source = root/'scripts/diagnose-accounts-webkit-version.sh'
fixture = root/'docs/verification/2026-09-12-accounts-webkit-version/fixture'
modules = pathlib.Path('/tmp/rcc-110-playwright-candidate/node_modules')
assert modules.is_dir(), 'Install the exact temporary candidate with --ignore-scripts first'
results=[]
for status, missing_backup, invalid_hash in [(0,False,False),(42,False,False),(0,True,False),(42,True,False),(0,False,True)]:
    with tempfile.TemporaryDirectory() as temporary:
        base=pathlib.Path(temporary); (base/'web').mkdir(); (base/'scripts').mkdir(); (base/'backup').mkdir()
        for name in ('package.json','pnpm-lock.yaml'): shutil.copyfile(root/'web'/name,base/'web'/name)
        (base/'web/node_modules').symlink_to(modules,target_is_directory=True)
        f=base/'docs/verification/2026-09-12-accounts-webkit-version/fixture';shutil.copytree(fixture,f)
        if invalid_hash:
            p=f/'candidate.sha256';s=p.read_text();p.write_text('0'*64+s[64:])
        bin_dir=base/'bin';bin_dir.mkdir();pnpm=bin_dir/'pnpm';pnpm.write_text('#!/bin/sh\nexit 0\n');pnpm.chmod(0o755)
        # The actual browser tracer is checked separately. This fixture only delegates to the runner.
        (base/'scripts/diagnose-accounts-webkit-signals.sh').write_text('#!/bin/bash\nbash "$1"\n')
        runner=base/'runner.sh'
        runner.write_text('touch case-entered\n'+('find "$TMPDIR" -type f -name package.json -delete\n' if missing_backup else '')+f'exit {status}\n')
        env=os.environ|{'PATH':str(bin_dir)+os.pathsep+os.environ['PATH'],'TMPDIR':str(base/'backup')+'/','RCC_SIGNAL_ARTIFACTS':str(base/'artifacts')}
        completed=subprocess.run(['bash',str(source),str(runner)],cwd=base,env=env,capture_output=True,text=True)
        expected=status if status else int(missing_backup or invalid_hash)
        assert completed.returncode==expected,completed.stdout+completed.stderr
        for name in ('package.json','pnpm-lock.yaml'):
            if missing_backup and name=='package.json':continue
            assert (base/'web'/name).read_bytes()==(root/'web'/name).read_bytes()
        assert not list((base/'backup').glob('rcc-110-dependencies.*'))
        assert (base/'case-entered').exists()!=invalid_hash
        if missing_backup:assert 'WebKit comparison dependency restoration failed' in completed.stderr
        results.append({'runner_exit':status,'missing_backup':missing_backup,'invalid_candidate_hash':invalid_hash,'supervisor_exit':completed.returncode,'expected':expected,'backup_removed':True,'passed':True})
print(json.dumps({'cases':results,'scope':'actual patch/hash/restore supervisor; pnpm and browser execution skipped; identity reads real locally installed candidate package'},indent=2))
