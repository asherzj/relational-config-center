#!/usr/bin/env bash
# Temporary #110 version comparison. Remove with its job/fixture before delivery.
set -Eeuo pipefail
[[ $# == 1 ]] || { echo 'Expected one generated diagnostic runner' >&2; exit 2; }
: "${RCC_SIGNAL_ARTIFACTS:?Expected signal artifact directory}"
fixture=docs/verification/2026-09-12-accounts-webkit-version/fixture
mkdir -p "$RCC_SIGNAL_ARTIFACTS"
sha256sum --check "$fixture/original.sha256" > "$RCC_SIGNAL_ARTIFACTS/dependencies-before-check.txt"
dependency_backup=$(mktemp -d "${TMPDIR:-/tmp}/rcc-110-dependencies.XXXXXX")
dependencies_backed_up=false
restore_dependencies() {
  local original_status=$?
  local restore_status=0
  set +e
  if [[ $dependencies_backed_up == true ]]; then
    cp -p -- "$dependency_backup/package.json" web/package.json || restore_status=1
    cp -p -- "$dependency_backup/pnpm-lock.yaml" web/pnpm-lock.yaml || restore_status=1
    sha256sum --check "$fixture/original.sha256" > "$RCC_SIGNAL_ARTIFACTS/dependencies-after-check.txt" || restore_status=1
  fi
  rm -rf -- "$dependency_backup" || restore_status=1
  if [[ $restore_status == 0 ]]; then
    printf 'dependency backup removed; original dependency files restored\n' > "$RCC_SIGNAL_ARTIFACTS/dependencies-cleanup.txt" || restore_status=1
  fi
  if [[ $restore_status != 0 ]]; then
    echo 'WebKit comparison dependency restoration failed' >&2
    if [[ $original_status == 0 ]]; then original_status=1; fi
  fi
  exit "$original_status"
}
trap restore_dependencies EXIT
cp -p -- web/package.json web/pnpm-lock.yaml "$dependency_backup/"
dependencies_backed_up=true
git apply --check "$fixture/candidate.patch"
git apply "$fixture/candidate.patch"
sha256sum --check "$fixture/candidate.sha256" > "$RCC_SIGNAL_ARTIFACTS/dependencies-candidate-check.txt"
cp -- "$fixture/candidate.patch" "$RCC_SIGNAL_ARTIFACTS/dependency-diff.patch"
pnpm --dir web install --frozen-lockfile
pnpm --dir web exec playwright install --with-deps webkit
node <<'IDENTITY' > "$RCC_SIGNAL_ARTIFACTS/comparison-identity.json"
const assert = require('node:assert/strict');
const { dirname, join } = require('node:path');
const location = require.resolve(process.cwd() + '/web/node_modules/playwright');
const playwright = require(location);
const version = require(process.cwd() + '/web/node_modules/playwright/package.json').version;
const core = dirname(require.resolve('playwright-core/package.json', { paths: [location] }));
const browser = require(join(core, 'browsers.json')).browsers.find(browser => browser.name === 'webkit');
assert.equal(version, '1.63.0');
assert.equal(require(join(core, 'package.json')).version, '1.63.0');
assert.equal(browser.revision, '2359');
assert.equal(browser.browserVersion, '26.6');
process.stdout.write(JSON.stringify({ playwright: version, core: require(join(core, 'package.json')).version, webkit: browser, executable: playwright.webkit.executablePath() }, null, 2) + '\n');
IDENTITY
bash scripts/diagnose-accounts-webkit-signals.sh "$1"
