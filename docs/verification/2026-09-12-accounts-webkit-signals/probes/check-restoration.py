"""Exercise the real supervisor's filesystem failure seam; no browser is mocked as passing."""
import json, os, pathlib, subprocess, tempfile
source = pathlib.Path(__file__).resolve().parents[4] / 'scripts/diagnose-accounts-webkit-signals.sh'
results = []
for case_status, lose_backup in [(0, False), (42, False), (0, True), (42, True)]:
    with tempfile.TemporaryDirectory() as temporary:
        root = pathlib.Path(temporary)
        launcher = root / 'pw_run.sh'
        launcher.write_text('#!/bin/sh\nexit 0\n'); launcher.chmod(0o751)
        module = root / 'web/node_modules/playwright'; module.mkdir(parents=True)
        (module / 'index.js').write_text('module.exports={webkit:{executablePath:()=>'+json.dumps(str(launcher))+'}};')
        bin_dir = root / 'bin'; bin_dir.mkdir()
        # Platform adapter for GNU stat; strace is only queried for its version in this fixture.
        (bin_dir / 'stat').write_text('#!/usr/bin/env python3\nimport os,sys\nprint(oct(os.stat(sys.argv[-1]).st_mode & 0o777)[2:])\n')
        (bin_dir / 'strace').write_text('#!/bin/sh\n[ "$1" = --version ] || exit 99\nprintf "restoration fixture; no browser tracing performed\\n"\n')
        for p in bin_dir.iterdir(): p.chmod(0o755)
        runner = root / 'runner.sh'
        runner.write_text(('rm -- '+"'"+str(launcher)+".rcc-110-original'\n" if lose_backup else '')+f'exit {case_status}\n')
        output = root / 'artifacts'
        env = os.environ | {'PATH':str(bin_dir)+os.pathsep+os.environ['PATH'], 'RCC_SIGNAL_ARTIFACTS':str(output)}
        completed = subprocess.run(['bash',str(source),str(runner)],cwd=root,env=env,capture_output=True,text=True)
        expected = case_status if case_status else int(lose_backup)
        assert completed.returncode == expected, completed.stderr
        if not lose_backup:
            assert launcher.read_text() == '#!/bin/sh\nexit 0\n'
            assert launcher.stat().st_mode & 0o777 == 0o751
            assert not pathlib.Path(str(launcher)+'.rcc-110-original').exists()
            for suffix in ('sha256','mode'):
                assert (output/f'launcher-before.{suffix}').read_bytes() == (output/f'launcher-after.{suffix}').read_bytes()
        else:
            assert 'WebKit diagnostic launcher restoration failed' in completed.stderr
        results.append({'runner_exit':case_status,'backup_removed_by_fixture':lose_backup,'supervisor_exit':completed.returncode,'expected':expected,'passed':True})
print(json.dumps({'restoration_cases':results},indent=2))
