#!/usr/bin/env bash
# Temporary #110 signal probe. Remove before delivery after this bounded run.
set -Eeuo pipefail
[[ $# == 1 ]] || { echo 'Expected one generated diagnostic runner' >&2; exit 2; }
: "${RCC_SIGNAL_ARTIFACTS:?Expected signal artifact directory}"
command -v strace >/dev/null
webkit_launcher=$(node -e 'process.stdout.write(require(process.cwd() + "/web/node_modules/playwright").webkit.executablePath())')
webkit_original="$webkit_launcher.rcc-110-original"
[[ -x "$webkit_launcher" && ! -e "$webkit_original" ]] || { echo 'Expected an unmodified installed WebKit launcher' >&2; exit 2; }
mkdir -p "$RCC_SIGNAL_ARTIFACTS"
sha256sum "$webkit_launcher" > "$RCC_SIGNAL_ARTIFACTS/launcher-before.sha256"
webkit_mode=$(stat -c '%a' "$webkit_launcher")
printf '%s\n' "$webkit_mode" > "$RCC_SIGNAL_ARTIFACTS/launcher-before.mode"
strace --version > "$RCC_SIGNAL_ARTIFACTS/strace-version.txt"
cp -p -- "$webkit_launcher" "$webkit_original"
restore_launcher() {
  local original_status=$?
  local restore_status=0
  set +e
  if cp -p -- "$webkit_original" "$webkit_launcher" && cmp -s -- "$webkit_original" "$webkit_launcher"; then
    sha256sum "$webkit_launcher" > "$RCC_SIGNAL_ARTIFACTS/launcher-after.sha256" || restore_status=1
    stat -c '%a' "$webkit_launcher" > "$RCC_SIGNAL_ARTIFACTS/launcher-after.mode" || restore_status=1
    cmp -s "$RCC_SIGNAL_ARTIFACTS/launcher-before.sha256" "$RCC_SIGNAL_ARTIFACTS/launcher-after.sha256" || restore_status=1
    cmp -s "$RCC_SIGNAL_ARTIFACTS/launcher-before.mode" "$RCC_SIGNAL_ARTIFACTS/launcher-after.mode" || restore_status=1
    rm -f -- "$webkit_original" || restore_status=1
  else
    restore_status=1
  fi
  if [[ $restore_status != 0 ]]; then
    echo 'WebKit diagnostic launcher restoration failed' >&2
    if [[ $original_status == 0 ]]; then original_status=1; fi
  fi
  exit "$original_status"
}
trap restore_launcher EXIT
cat > "$webkit_launcher" <<'WRAPPER'
#!/bin/sh
# Trace only the original browser process tree; no argv/env or read/write payloads.
exec strace -f -ttt --decode-pids=comm -e trace=exit,exit_group,wait4,waitid,kill,tgkill -e signal=all -o "${RCC_SIGNAL_ARTIFACTS:?}/webkit-$$" "$0.rcc-110-original" "$@"
WRAPPER
bash "$1"
