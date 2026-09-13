// Application-driver contracts only. These controls never launch a browser or reproduce #113.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync, spawnSync } = require('node:child_process');
const driver = require('./diagnose-a11y-webkit-repeat.cjs');
const repo = path.resolve(__dirname, '..');
const original = execFileSync('git', ['show', `${driver.BASE}:scripts/browser-acceptance.sh`], { cwd: repo, encoding: 'utf8' });
const originalBusiness = execFileSync('git', ['show', `${driver.BASE}:web/e2e/browser-accessibility.cjs`], { cwd: repo, encoding: 'utf8' });
const generated = driver.generateRunner(original);
const compact = driver.generate(originalBusiness, 12);
function fixture(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rcc-a11y-driver-test-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

test('source-derived compact removes exactly one original drawer block and preserves the remaining CR/LF steps and deadlines', t => {
  const directory=fixture(t);
  fs.writeFileSync(path.join(directory,'compact.cjs'),compact.script);
  fs.writeFileSync(path.join(directory,'runner.sh'),generated);
  execFileSync(process.execPath,['--check',path.join(directory,'compact.cjs')]);
  execFileSync('bash',['-n',path.join(directory,'runner.sh')]);
  assert.deepEqual(compact.provenance.retainedBusiness,{spans:[{firstLine:209,lastLine:221},{firstLine:250,lastLine:370}],sha256:driver.sha256([...originalBusiness.split('\n').slice(208,221),...originalBusiness.split('\n').slice(249,370)].join('\n')+'\n'),byteIdentityExcludingHostPhaseInsertions:true});
  assert.equal(compact.provenance.targetOriginalLine,355);
  assert.equal(compact.provenance.leaveOriginalLine,360);
  assert.deepEqual(compact.provenance.deleted.map(x=>[x.firstLine,x.lastLine]),[[139,208],[222,249],[371,429]]);
  assert.match(compact.script,/page.setDefaultTimeout\(15000\)/);
  assert.match(compact.script,/page.setDefaultNavigationTimeout\(20000\)/);
  assert.match(compact.script,/await button\('note 申请值：转换为 LF 再编辑'\).click\(\)/);
  assert.match(compact.script,/await button\('放弃修改并离开'\).click\(\)/);
  assert.doesNotMatch(compact.script,/page.route\(|clickWithDiagnostics|force:|native-trace|strace|elfFiles/);
  assert.equal(generated.replace(driver.COMPACT_INVOCATION,driver.ORIGINAL_INVOCATION).replace(`${driver.IDENTITY_CHECK}\n`,''),original);
  assert.equal(generated.split('\n').filter(x=>x.includes('run-with-timeout.cjs')&&x.includes(' 2400 ')).length,2);
});

test('restoring only the named block and mechanical check accounting reproduces D09 byte-for-byte', t => {
  const directory = fixture(t);
  const previousFile = path.join(directory, 'd09.cjs');
  fs.writeFileSync(previousFile, execFileSync('git', ['show', 'e616f8c7c446da686b67a2d03b813e5e467499c7:scripts/diagnose-a11y-webkit-repeat.cjs'], { cwd: repo }));
  const previous = require(previousFile).generate(originalBusiness, 12).script;
  const removed = originalBusiness.split('\n').slice(221,249).join('\n') + '\n';
  assert.equal(driver.sha256(removed), 'a0abd1efca501e633cd7a9911ea9e7b704b3a5bbd76bbd2084e4ab692672b94c');
  assert.equal(compact.provenance.deleted[1].sha256, driver.sha256(removed));
  const boundary = "    assert.equal(await note.getAttribute('readonly'), '');\n";
  assert.equal(compact.script.split(boundary).length, 2);
  const restored = compact.script.replace(boundary, boundary + removed)
    .replace('assert.equal(currentRound.checks, 3);', 'assert.equal(currentRound.checks, 4);')
    .replace("omitted: 'original checks 1, 2, 3, 7;", "omitted: 'original checks 1, 2, 7;");
  assert.equal(restored, previous);
  const previousWorkflow = execFileSync('git', ['show', 'e616f8c7c446da686b67a2d03b813e5e467499c7:.github/workflows/ci.yml'], { cwd: repo, encoding: 'utf8' });
  assert.equal(fs.readFileSync(path.join(repo, '.github/workflows/ci.yml'), 'utf8'), previousWorkflow);
});

test('changed source and invalid round counts fail before execution', () => {
  assert.throws(()=>driver.generate(originalBusiness+'\n',12));
  assert.throws(()=>driver.generateRunner(original+'\n'));
  for(const count of [0,-1,13,NaN,Infinity,1.5,'12',undefined]) assert.throws(()=>driver.generate(originalBusiness,count));
});

test('artifact paths reject absent, unsafe, occupied and symlink destinations', t => {
  const directory=fixture(t);
  for(const value of [undefined,'',' ','/','a\nb','a\0b']) assert.throws(()=>driver.artifactDirectory(value));
  fs.writeFileSync(path.join(directory,'occupied'),'existing');
  assert.throws(()=>driver.artifactDirectory(directory));
  const link=path.join(directory,'link');fs.symlinkSync(directory,link);
  assert.throws(()=>driver.artifactDirectory(link));
  assert.equal(driver.artifactDirectory(path.join(directory,'new')),path.join(directory,'new'));
});

test('only the LF phase plus original timeout and not-stable text is the exact symptom', () => {
  const error={name:'TimeoutError',message:'locator.click: Timeout 15000ms exceeded. element is not stable'};
  assert.equal(driver.classify(error,'original-LF-355'),'exact-original-LF-notstable-symptom');
  for(const phase of ['original-leave-360','CR-path','post-LF-assertions']) assert.equal(driver.classify(error,phase),'other-failure');
  for(const message of ['locator.click: Timeout 15000ms exceeded.','Target closed','locator.click: Timeout 30000ms exceeded. element is not stable']) assert.equal(driver.classify({...error,message},'original-LF-355'),'other-failure');
  assert.equal(driver.classify(null,'final-assertions'),'not-reproduced-in-bounded-experiment');
  assert.equal(driver.classify(error,'setup'),'incomplete-setup');
  for (const phase of ['round-start-observation','round-result-observation']) assert.equal(driver.classify(error,phase),'incomplete-observation');
});

for(const fail of [0,1,6,12,'pageerror','append']) test(`actual generated orchestration stops at controlled external boundary ${fail}`, async t => {
  const directory=fixture(t);
  let loop=compact.script.slice(compact.script.indexOf('    for (let round = 1;'),compact.script.indexOf("    phase = 'final-assertions';"));
  const a=loop.indexOf('    // Narrow drawer,');
  const b=loop.indexOf("    currentRound.elapsedMs =",a);
  // Replace the entire real-browser business boundary for this host-only control.
  // No browser, product service or simulated browser fault is involved.
  loop=loop.slice(0,a)+`if(round===${JSON.stringify(fail)}) throw new Error('controlled external boundary failure'); checks.push(1,2,3);currentRound.lfPassed=true;if(${JSON.stringify(fail)}==='pageerror')pageErrors.push('controlled page-error record');\n`+loop.slice(b);
  const fn=new Function('fs','rootOutput','assert',`return (async()=>{let output,phase,currentRound;const roundResults=[],checks=[],pageErrors=[],maxRounds=12;try{${loop}}catch(error){return {error:error.message,roundResults};}return {roundResults};})()`);
  const recording = { ...fs.promises, appendFile: async (file, data) => {
    if (fail === 'append' && JSON.parse(data).state === 'passed') throw new Error('controlled event-write failure');
    return fs.promises.appendFile(file, data);
  } };
  const result=await fn(recording,directory,assert);
  assert.equal(result.roundResults.length,typeof fail==='string'?1:fail||12);
  assert.equal(result.roundResults.filter(x=>x.state==='passed').length,typeof fail==='string'?0:fail?fail-1:12);
  assert.equal(Boolean(result.error),Boolean(fail));
});

for(const seconds of [0,1921]) test(`original complete-case admission at elapsed ${seconds}s`, t => {
  const directory=fixture(t);
  const block=driver.COMPACT_INVOCATION.slice(0,driver.COMPACT_INVOCATION.lastIndexOf('\ndone\nfi'));
  const script=`set -Eeuo pipefail\nSECONDS=${seconds}\nartifact_root=$1\nrepo_root=/original\nbrowser_engine=webkit\nrun_browser_suite(){ printf '%s\\n' "$4" > "$artifact_root/call"; }\n${block}\n`;
  const result=spawnSync('bash',['-s','--',directory],{input:script,encoding:'utf8'});
  assert.equal(result.status,seconds?125:0,result.stderr);
  assert.equal(fs.existsSync(path.join(directory,'call')),!seconds);
  if(!seconds)assert.equal(fs.readFileSync(path.join(directory,'call'),'utf8').trim(),'420');
});

test('summary preserves exact, other, incomplete, cleanup and missing-round distinctions', t => {
  const directory=fixture(t), output=path.join(directory,'browser-accessibility/webkit');fs.mkdirSync(output,{recursive:true});
  const file=path.join(output,'result.json');
  const rounds=Array.from({length:12},(_,i)=>({round:i+1,state:'passed',checks:3,lfPassed:true}));
  const result={ok:true,browserVersion:'26.6',pageErrors:[],cleanup:{remainingRows:0},checks:Array(36).fill({}),compact:{phase:'final-assertions',rounds}};
  fs.writeFileSync(path.join(directory,'run.txt'),'cleanup verified: true\n');
  const write=()=>fs.writeFileSync(file,JSON.stringify(result));write();
  assert.equal(driver.summarize(directory,0).outcome,'not-reproduced-in-bounded-experiment');
  result.ok=false;result.failure={name:'TimeoutError',message:'locator.click: Timeout 15000ms exceeded. element is not stable'};result.compact.phase='original-LF-355';write();
  assert.equal(driver.summarize(directory,1).outcome,'exact-original-LF-notstable-symptom');
  result.compact.phase='original-leave-360';write();assert.equal(driver.summarize(directory,1).outcome,'other-failure');
  assert.throws(()=>driver.summarize(directory,0),/incomplete experiment/);
  result.cleanup={error:'cleanup failed'};write();assert.equal(driver.summarize(directory,1).outcome,'incomplete-cleanup');
  fs.unlinkSync(file);fs.writeFileSync(path.join(output,'round-events.jsonl'),JSON.stringify({round:1,state:'started'})+'\n');
  const incomplete=driver.summarize(directory,124);assert.equal(incomplete.outcome,'incomplete-experiment');
  assert.equal(incomplete.rounds[0].state,'interrupted');assert.ok(incomplete.rounds.slice(1).every(x=>x.state==='unexecuted'));
});

test('ordinary workflow jobs remain unchanged and the extra job contains no native observer setup', () => {
  const previous=execFileSync('git',['show','9da16c6d06cba62dbf9773255f83d8c6a30be3ae:.github/workflows/ci.yml'],{cwd:repo,encoding:'utf8'});
  const current=fs.readFileSync(path.join(repo,'.github/workflows/ci.yml'),'utf8');
  assert.equal(current.slice(0,current.indexOf('  # Temporary #113/')),previous.slice(0,previous.indexOf('  # Temporary #113/')));
  assert.doesNotMatch(current,/strace|gcc|native-trace/);
  assert.equal(fs.existsSync(path.join(repo,'scripts/diagnose-a11y-native-trace.cjs')),false);
  assert.equal(fs.existsSync(path.join(repo,'scripts/diagnose-a11y-native-trace.test.cjs')),false);
});
test('source mismatch prevents the external execution boundary and removes only its snapshot', async t => {
  const directory = fixture(t);
  const external = path.join(directory, 'do-not-remove');
  fs.writeFileSync(external, 'existing data');
  const marker = path.join(directory, 'executed');
  const result = await driver.withOwnedSnapshot(directory, async snapshot => {
    for (const file of Object.keys(driver.HASHES)) {
      const destination = path.join(snapshot, file);
      fs.mkdirSync(path.dirname(destination), { recursive: true });
      fs.writeFileSync(destination, execFileSync('git', ['show', `${driver.BASE}:${file}`], { cwd: repo }));
    }
    fs.appendFileSync(path.join(snapshot, 'web/e2e/browser-accessibility.cjs'), '\n');
    driver.verifySource(snapshot);
    return driver.runCommand(process.execPath, ['-e', 'require("node:fs").writeFileSync(process.argv[1],"started")', marker]);
  });
  assert.equal(result.status, 1);
  assert.match(result.failure.message, /original source mismatch/);
  assert.equal(result.removed, true);
  assert.equal(fs.existsSync(marker), false);
  assert.equal(fs.readFileSync(external, 'utf8'), 'existing data');
});

test('external-command failure survives successful snapshot cleanup', async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory, async snapshot => {
    fs.writeFileSync(path.join(snapshot, 'owned'), 'temporary');
    return driver.runCommand(process.execPath, ['-e', 'process.exit(7)']);
  });
  assert.equal(result.status, 7);
  assert.equal(result.removed, true);
});

for (const originalStatus of [0, 7]) test(`filesystem cleanup failure cannot replace original status ${originalStatus} with success`, async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory,
    () => driver.runCommand(process.execPath, ['-e', `process.exit(${originalStatus})`]),
    () => { throw Object.assign(new Error('controlled filesystem denial'), { code: 'EACCES' }); });
  assert.equal(result.status, originalStatus || 1);
  assert.equal(result.removed, false);
  assert.equal(result.cleanupError.code, 'EACCES');
});

test('post-command evidence failure retains an already-failed external exit status', async t => {
  const directory = fixture(t);
  const result = await driver.withOwnedSnapshot(directory, async () => {
    const exitStatus = await driver.runCommand(process.execPath, ['-e', 'process.exit(7)']);
    throw Object.assign(new Error('controlled invalid evidence'), { exitStatus });
  });
  assert.equal(result.status, 7);
  assert.equal(result.removed, true);
  assert.match(result.failure.message, /invalid evidence/);
});

test('identity gate resolves pnpm package symlinks and rejects the wrong installed core', t => {
  const directory = fixture(t);
  const modules = path.join(directory, 'web/node_modules');
  const playwright = path.join(modules, '.pnpm/playwright@1.63.0/node_modules/playwright');
  const core = path.join(modules, '.pnpm/playwright-core@1.63.0/node_modules/playwright-core');
  const executable = path.join(directory, 'webkit-2359/pw_run.sh');
  fs.mkdirSync(playwright, { recursive: true });
  fs.mkdirSync(core, { recursive: true });
  fs.mkdirSync(path.dirname(executable));
  fs.writeFileSync(executable, 'external executable fixture; never launched');
  fs.writeFileSync(path.join(playwright, 'package.json'), '{"version":"1.63.0","main":"index.js"}');
  fs.writeFileSync(path.join(playwright, 'index.js'), `module.exports={webkit:{executablePath:()=>${JSON.stringify(executable)}}};`);
  const corePackage = path.join(core, 'package.json');
  fs.writeFileSync(corePackage, '{"version":"1.63.0"}');
  fs.writeFileSync(path.join(core, 'browsers.json'), '{"browsers":[{"name":"webkit","revision":"2359","browserVersion":"26.6"}]}');
  fs.symlinkSync(playwright, path.join(modules, 'playwright'));
  fs.symlinkSync(core, path.join(path.dirname(playwright), 'playwright-core'));
  // Only the host-platform precondition is removed for this portable package
  // boundary test. The CI driver retains both Linux and AMD64 assertions.
  const identity = driver.IDENTITY_CHECK.split("<<'RCC_D09_IDENTITY'\n")[1].split('\nRCC_D09_IDENTITY')[0]
    .replace("assert.equal(process.platform, 'linux');", '').replace("assert.equal(process.arch, 'x64');", '');
  const invoke = () => spawnSync(process.execPath, ['-', directory, directory], { input: identity, encoding: 'utf8' });
  const valid = invoke();
  assert.equal(valid.status, 0, valid.stderr);
  const identityFile = path.join(directory, 'browser-identity.json');
  assert.equal(JSON.parse(fs.readFileSync(identityFile)).core, '1.63.0');
  fs.unlinkSync(identityFile);
  fs.writeFileSync(corePackage, '{"version":"1.62.1"}');
  const invalid = invoke();
  assert.notEqual(invalid.status, 0);
  assert.equal(fs.existsSync(identityFile), false);
});

for (const signal of ['SIGINT', 'SIGTERM']) {
  for (const phase of ['setup', 'cleanup']) test(`${signal} during synchronous ${phase} removes the owned snapshot`, async t => {
    const directory = fixture(t);
    const ready = path.join(directory, 'ready');
    const release = path.join(directory, 'release');
    const snapshotFile = path.join(directory, 'snapshot');
    const resultFile = path.join(directory, 'result.json');
    const blocker = `const fs=require('node:fs');fs.writeFileSync(${JSON.stringify(ready)},'ready');const until=Date.now()+3000;while(!fs.existsSync(${JSON.stringify(release)})&&Date.now()<until)Atomics.wait(new Int32Array(new SharedArrayBuffer(4)),0,0,10);`;
    const childSource = `
      const fs=require('node:fs');
      const {execFileSync}=require('node:child_process');
      const {withOwnedSnapshot}=require(${JSON.stringify(require.resolve('./diagnose-a11y-webkit-repeat.cjs'))});
      const block=()=>execFileSync(process.execPath,['-e',${JSON.stringify(blocker)}]);
      withOwnedSnapshot(${JSON.stringify(directory)}, snapshot=>{
        fs.writeFileSync(${JSON.stringify(snapshotFile)},snapshot);
        fs.writeFileSync(snapshot+'/owned','temporary');
        if(${JSON.stringify(phase)}==='setup')block();
        return 0;
      },(snapshot,options)=>{
        if(${JSON.stringify(phase)}==='cleanup')block();
        fs.rmSync(snapshot,options);
      }).then(result=>{
        fs.writeFileSync(${JSON.stringify(resultFile)},JSON.stringify(result));
        process.exitCode=result.status;
      });`;
    const child = require('node:child_process').spawn(process.execPath, ['-e', childSource], { stdio: 'ignore' });
    const completion = new Promise((resolve, reject) => {
      child.once('error', reject);
      child.once('close', (code, exitedSignal) => resolve({ code, signal: exitedSignal }));
    });
    t.after(() => { if (fs.existsSync(directory)) fs.writeFileSync(release, 'released'); child.kill('SIGKILL'); });
    const deadline = Date.now() + 2000;
    while (!fs.existsSync(ready) && Date.now() < deadline) await new Promise(resolve => setTimeout(resolve, 10));
    assert.ok(fs.existsSync(ready), 'child must enter the real synchronous boundary before signaling');
    const snapshot = fs.readFileSync(snapshotFile, 'utf8');
    assert.ok(fs.existsSync(snapshot));
    child.kill(signal);
    // Let the signal reach the blocked driver before releasing its sync child.
    await new Promise(resolve => setTimeout(resolve, 30));
    fs.writeFileSync(release, 'released');
    const exited = await completion;
    assert.equal(fs.existsSync(snapshot), false, 'handled interruption must not leave its source snapshot');
    assert.deepEqual(exited, { code: signal === 'SIGINT' ? 130 : 143, signal: null });
    const result = JSON.parse(fs.readFileSync(resultFile, 'utf8'));
    assert.equal(result.removed, true);
    assert.equal(result.cleanupError, null);
  });
}
