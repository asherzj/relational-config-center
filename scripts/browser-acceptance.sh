#!/usr/bin/env bash

set -Eeuo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
run_id="rcc-browser-${EPOCHSECONDS:-$(date +%s)}-$$"
artifact_root=${RCC_E2E_ARTIFACTS:-"${TMPDIR:-/tmp}/${run_id}"}
runtime_dir=""
mysql_container="${run_id}-mysql"
mysql_volume="${run_id}-mysql-data"
admin_pid=""
web_pid=""
admin_port=""
web_port=""
docker_resources_started=false
active_pid=""
run_completed=false

if [[ -d "$artifact_root" && -n $(ls -A "$artifact_root" 2>/dev/null) ]]; then
  printf 'artifact directory must be new or empty: %s\n' "$artifact_root" >&2
  exit 2
fi
mkdir -p "$artifact_root/unsaved-changes" "$artifact_root/rule-clarity" "$artifact_root/write-recovery" "$artifact_root/operation-coverage" "$artifact_root/complex-fields" "$artifact_root/browser-accessibility" "$artifact_root/release-workflow"
umask 077

for command in docker node pnpm go curl od tr grep sort cmp; do
  if ! command -v "$command" >/dev/null 2>&1; then
    printf 'required command is unavailable: %s\n' "$command" >&2
    exit 2
  fi
done

case ${RCC_E2E_SUITE:-all} in
  all|unsaved-changes|rule-clarity|write-recovery|operation-coverage|complex-fields|browser-accessibility|release-workflow|rollback-reason|field-interactions) ;;
  *) printf 'unknown browser suite: %s\n' "$RCC_E2E_SUITE" >&2; exit 2 ;;
esac

runtime_dir=$(mktemp -d "${TMPDIR:-/tmp}/rcc-browser-runtime.XXXXXX")

random_secret() {
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
}

mysql_root_password=$(random_secret)
mysql_password=$(random_secret)
mysql_env="$runtime_dir/mysql.env"
{
  printf 'MYSQL_ROOT_PASSWORD=%s\n' "$mysql_root_password"
  printf 'MYSQL_DATABASE=rcc\n'
  printf 'MYSQL_USER=rcc_admin\n'
  printf 'MYSQL_PASSWORD=%s\n' "$mysql_password"
} > "$mysql_env"

capture_mysql_state() {
  local target=$1
  if run_timeout 5 docker inspect "$mysql_container" >/dev/null 2>&1; then
    run_timeout 5 docker logs "$mysql_container" > "$artifact_root/mysql.log" 2>&1 || true
    run_timeout 5 docker exec "$mysql_container" sh -c \
      'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE" -e "
        SELECT CONCAT(
          (SELECT COUNT(*) FROM stage1_acceptance_items), \"|\",
          COALESCE((SELECT MIN(id) FROM stage1_acceptance_items), 0), \"|\",
          COALESCE((SELECT MAX(id) FROM stage1_acceptance_items), 0), \"|\",
          (SELECT COUNT(*) FROM rcc_query_policies WHERE code LIKE \"stage2_unsaved_query_%\"), \"|\",
          (SELECT CONCAT(query_policy_code, \"|\", mutation_policy_code, \"|\", enabled)
             FROM rcc_table_policies WHERE table_name = \"stage1_acceptance_items\"), \"|\",
          (SELECT status FROM rcc_mutation_policies WHERE code = \"stage1_mutation_v1\")
        );"' > "$target" 2>&1 || true
  fi
}

check_persisted_secrets() {
  local matches="$runtime_dir/secret-leak-files.txt"
  local errors="$runtime_dir/secret-scan-error.txt"
  local current_matches="$runtime_dir/secret-current-matches.txt"
  local current_errors="$runtime_dir/secret-current-errors.txt"
  local roots=("$artifact_root")
  local grep_status
  local leak=false
  local scan_error=false
  local secret
  if [[ -d "$repo_root/web/dist" ]]; then roots+=("$repo_root/web/dist"); fi
  : > "$matches"
  : > "$errors"
  for secret in "$mysql_password" "$mysql_root_password"; do
    set +e
    grep -R -F -l -- "$secret" "${roots[@]}" > "$current_matches" 2> "$current_errors"
    grep_status=$?
    set -e
    case $grep_status in
      0) cat "$current_matches" >> "$matches"; leak=true ;;
      1) ;;
      *) cat "$current_errors" >> "$errors"; scan_error=true ;;
    esac
  done
  rm -f "$current_matches" "$current_errors"
  if [[ $leak == true ]]; then
    sort -u "$matches" > "$artifact_root/secret-leak-files.txt"
    printf 'a generated credential was found in persisted output\n' >&2
  fi
  if [[ $scan_error == true ]]; then
    mv "$errors" "$artifact_root/secret-scan-error.txt"
    printf 'could not verify persisted output for generated credentials\n' >&2
  fi
  rm -f "$matches" "$errors"
  [[ $leak == false && $scan_error == false ]]
}

stop_service() {
  local pid=$1
  local deadline=$((SECONDS + 4))
  kill "$pid" >/dev/null 2>&1 || true
  while kill -0 "$pid" >/dev/null 2>&1 && (( SECONDS < deadline )); do sleep 0.1; done
  kill -KILL "$pid" >/dev/null 2>&1 || true
  wait "$pid" >/dev/null 2>&1 || true
}

cleanup() {
  local status=$?
  local cleanup_ok=true
  # Some older Bash expansion errors enter EXIT with status 0. Success also
  # requires every selected suite and the final fixture checks to complete.
  if [[ $run_completed != true && $status == 0 ]]; then status=1; fi
  trap - EXIT INT TERM
  if [[ -n "$active_pid" ]]; then stop_service "$active_pid"; active_pid=""; fi
  if [[ -n "$web_pid" ]]; then stop_service "$web_pid"; fi
  if [[ -n "$admin_pid" ]]; then stop_service "$admin_pid"; fi
  capture_mysql_state "$artifact_root/database-final.txt"
  if ! check_persisted_secrets; then cleanup_ok=false; fi
  if [[ -n "$web_port" ]] && curl --silent --max-time 1 "http://127.0.0.1:$web_port" >/dev/null 2>&1; then
    printf 'Web listener still responds after cleanup: %s\n' "$web_port" >&2
    cleanup_ok=false
  fi
  if [[ -n "$admin_port" ]] && curl --silent --max-time 1 "http://127.0.0.1:$admin_port/health/live" >/dev/null 2>&1; then
    printf 'Admin listener still responds after cleanup: %s\n' "$admin_port" >&2
    cleanup_ok=false
  fi
  if [[ $docker_resources_started == true ]]; then
    run_timeout 10 docker rm --force --volumes "$mysql_container" >/dev/null 2>&1 || true
    run_timeout 10 docker volume rm "$mysql_volume" >/dev/null 2>&1 || true
    if ! run_timeout 5 docker info >/dev/null 2>&1; then
      printf 'Docker unavailable; cleanup could not be verified for %s and %s\n' \
        "$mysql_container" "$mysql_volume" >&2
      cleanup_ok=false
    else
      if run_timeout 5 docker inspect "$mysql_container" >/dev/null 2>&1; then
        printf 'MySQL container survived cleanup: %s\n' "$mysql_container" >&2
        cleanup_ok=false
      fi
      if run_timeout 5 docker volume inspect "$mysql_volume" >/dev/null 2>&1; then
        printf 'MySQL volume survived cleanup: %s\n' "$mysql_volume" >&2
        cleanup_ok=false
      fi
    fi
  fi
  if [[ $cleanup_ok != true && $status == 0 ]]; then status=1; fi
  {
    printf 'exit status: %s\n' "$status"
    printf 'finished at: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
    printf 'cleanup verified: %s\n' "$cleanup_ok"
    printf 'container name: %s\nvolume name: %s\n' "$mysql_container" "$mysql_volume"
  } >> "$artifact_root/run.txt"
  if [[ -n "$runtime_dir" ]]; then rm -rf "$runtime_dir"; fi
  printf 'artifact directory: %s\n' "$artifact_root"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

wait_for_mysql() {
  local deadline=$((SECONDS + 90))
  while (( SECONDS < deadline )); do
    if ! run_timeout 5 docker inspect "$mysql_container" >/dev/null 2>&1; then
      printf 'MySQL container disappeared before readiness\n' >&2
      return 1
    fi
    if [[ $(run_timeout 5 docker inspect -f '{{.State.Running}}' "$mysql_container") != true ]]; then
      printf 'MySQL exited before readiness\n' >&2
      run_timeout 5 docker logs "$mysql_container" >&2 || true
      return 1
    fi
    if run_timeout 5 docker exec "$mysql_container" sh -c \
      'test "$(cat /proc/1/comm)" = mysqld && MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysqladmin ping -h 127.0.0.1 -uroot --silent' \
      >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf 'MySQL did not become ready within 90s\n' >&2
  run_timeout 5 docker logs "$mysql_container" >&2 || true
  return 1
}

wait_for_http() {
  local name=$1
  local url=$2
  local pid=$3
  local log=$4
  local deadline=$((SECONDS + 45))
  while (( SECONDS < deadline )); do
    if ! kill -0 "$pid" >/dev/null 2>&1; then
      printf '%s exited before readiness\n' "$name" >&2
      tail -n 100 "$log" >&2 || true
      return 1
    fi
    if curl --fail --silent --show-error --max-time 2 "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  printf '%s did not become ready within 45s: %s\n' "$name" "$url" >&2
  tail -n 100 "$log" >&2 || true
  return 1
}

free_port() {
  node -e 'const net=require("node:net");const s=net.createServer();s.unref();s.listen(0,"127.0.0.1",()=>{console.log(s.address().port);s.close()})'
}

run_timeout() {
  local seconds=$1
  local status
  shift
  RCC_TIMEOUT_KILL_GRACE_MS=1000 \
    node "$repo_root/scripts/run-with-timeout.cjs" "$seconds" "$@" <&0 &
  active_pid=$!
  if wait "$active_pid"; then status=0; else status=$?; fi
  active_pid=""
  return "$status"
}

run_logged() {
  local seconds=$1
  local log=$2
  local status
  shift 2
  if run_timeout "$seconds" "$@" > "$log" 2>&1; then status=0; else status=$?; fi
  cat "$log"
  return "$status"
}

printf 'run id: %s\n' "$run_id" > "$artifact_root/run.txt"
printf 'container name: %s\nvolume name: %s\n' "$mysql_container" "$mysql_volume" >> "$artifact_root/run.txt"
printf 'started at: %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" >> "$artifact_root/run.txt"
run_timeout 15 docker version --format 'docker client={{.Client.Version}} server={{.Server.Version}}' \
  >> "$artifact_root/run.txt" 2> "$artifact_root/docker-check.log"
node --version >> "$artifact_root/run.txt"
pnpm --version >> "$artifact_root/run.txt"
go version >> "$artifact_root/run.txt"

printf 'Installing pinned Web dependencies and required Playwright browsers...\n'
run_logged 300 "$artifact_root/dependencies-install.log" \
  pnpm --dir "$repo_root/web" install --frozen-lockfile
browser_engines=${RCC_E2E_ENGINES:-${RCC_E2E_ENGINE:-chromium}}
browser_engine_list=()
if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == browser-accessibility || ${RCC_E2E_SUITE:-all} == release-workflow || ${RCC_E2E_SUITE:-all} == field-interactions ]]; then
  engine_ifs=$IFS
  IFS=,
  read -r -a browser_engine_list <<< "$browser_engines"
  IFS=$engine_ifs
  for browser_engine in "${browser_engine_list[@]}"; do
    case $browser_engine in chromium|firefox|webkit) ;; *) printf 'unknown browser engine: %s\n' "$browser_engine" >&2; exit 2 ;; esac
  done
fi
if [[ ${RCC_E2E_SUITE:-all} == browser-accessibility || ${RCC_E2E_SUITE:-all} == release-workflow || ${RCC_E2E_SUITE:-all} == field-interactions ]]; then
  playwright_install_targets=("${browser_engine_list[@]}")
else
  playwright_install_targets=(chromium)
  if [[ ${RCC_E2E_SUITE:-all} == all ]]; then
    for browser_engine in "${browser_engine_list[@]}"; do
      target_present=false
      for install_target in "${playwright_install_targets[@]}"; do
        if [[ $install_target == "$browser_engine" ]]; then target_present=true; break; fi
      done
      if [[ $target_present == false ]]; then playwright_install_targets+=("$browser_engine"); fi
    done
  fi
fi
if [[ $(uname -s) == Linux && ${CI:-} == true ]]; then
  run_logged 600 "$artifact_root/chromium-install.log" \
    pnpm --dir "$repo_root/web" exec playwright install --with-deps "${playwright_install_targets[@]}"
else
  run_logged 600 "$artifact_root/chromium-install.log" \
    pnpm --dir "$repo_root/web" exec playwright install "${playwright_install_targets[@]}"
fi

printf 'Building the Web preview artifact...\n'
run_logged 180 "$artifact_root/web-build.log" pnpm --dir "$repo_root/web" build

printf 'Verifying timeout cleanup against a TERM-resistant descendant...\n'
run_logged 15 "$artifact_root/timeout-test.log" \
  node --test "$repo_root/scripts/run-with-timeout.test.cjs"

printf 'Starting disposable MySQL 8.4 container %s...\n' "$mysql_container"
docker_resources_started=true
run_timeout 60 docker volume create --label rcc.browser-acceptance="$run_id" "$mysql_volume" \
  > "$artifact_root/mysql-volume.txt"
mysql_bind_port=$(free_port)
mysql_server_arguments=(mysql:8.4)
if [[ -n ${RCC_E2E_MYSQL_SQL_MODE:-} ]]; then
  mysql_server_arguments+=("--sql-mode=$RCC_E2E_MYSQL_SQL_MODE")
fi
run_timeout 300 docker run --detach --name "$mysql_container" \
  --label rcc.browser-acceptance="$run_id" \
  --env-file "$mysql_env" \
  --publish "127.0.0.1:$mysql_bind_port:3306" \
  --mount "type=volume,source=$mysql_volume,target=/var/lib/mysql" \
  "${mysql_server_arguments[@]}" > "$artifact_root/mysql-container-id.txt" 2> "$artifact_root/mysql-start.log"
wait_for_mysql

mysql_endpoint=$(run_timeout 5 docker port "$mysql_container" 3306/tcp)
mysql_port=${mysql_endpoint##*:}
if [[ ! $mysql_port =~ ^[0-9]+$ ]]; then
  printf 'could not determine the dynamic MySQL port: %s\n' "$mysql_endpoint" >&2
  exit 1
fi

load_sql() {
  local sql_file=$1
  local sql_output="$runtime_dir/sql-load.out"
  local status
  printf 'Loading %s...\n' "${sql_file#$repo_root/}"
  if run_timeout 60 docker exec --interactive "$mysql_container" sh -c \
    'MYSQL_PWD="$MYSQL_PASSWORD" mysql -u"$MYSQL_USER" "$MYSQL_DATABASE"' \
    < "$sql_file" > "$sql_output" 2>&1; then status=0; else status=$?; fi
  tee -a "$artifact_root/sql-load.log" < "$sql_output"
  return "$status"
}

load_sql "$repo_root/deploy/mysql/init/001-schema.sql"
load_sql "$repo_root/deploy/mysql/local-fixture/002-notification-templates.sql"
load_sql "$repo_root/docs/verification/fixtures/stage1_acceptance.sql"
load_sql "$repo_root/web/e2e/fixtures/stage1-policies.sql"
load_sql "$repo_root/admin/cmd/admin/testdata/014-batch-browser.sql"
load_sql "$repo_root/web/e2e/fixtures/release-rollbacks.sql"

printf 'Granting the disposable Admin account publication metadata access...\n'
run_timeout 30 docker exec --interactive "$mysql_container" sh -c \
  'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot "$MYSQL_DATABASE"' \
  < "$repo_root/admin/cmd/admin/testdata/013-publication-grants.sql" \
  > "$artifact_root/publication-grants.log" 2>&1

capture_fixture_rows() {
  local target=$1
  run_timeout 10 docker exec "$mysql_container" sh -c \
    'MYSQL_PWD="$MYSQL_PASSWORD" mysql --batch --skip-column-names -u"$MYSQL_USER" "$MYSQL_DATABASE" -e "
      SELECT CONCAT_WS(\"|\",
        id,
        HEX(name),
        IF(category IS NULL, \"NULL\", CONCAT(\"HEX:\", HEX(category))),
        IF(note IS NULL, \"NULL\", CONCAT(\"HEX:\", HEX(note))),
        HEX(state),
        priority,
        HEX(created_by),
        DATE_FORMAT(created_at, \"%Y%m%d%H%i%s.%f\"),
        HEX(updated_by),
        DATE_FORMAT(updated_at, \"%Y%m%d%H%i%s.%f\")
      ) FROM stage1_acceptance_items ORDER BY id;"' > "$target"
}

capture_fixture_rows "$artifact_root/fixture-before.tsv"

admin_port=$(free_port)
web_port=$(free_port)
if [[ $admin_port == "$web_port" ]]; then web_port=$(free_port); fi
admin_url="http://127.0.0.1:$admin_port"
web_url="http://127.0.0.1:$web_port"

printf 'Building and starting Admin on a dynamic loopback port...\n'
run_logged 300 "$artifact_root/admin-build.log" \
  go -C "$repo_root/admin" build -o "$runtime_dir/admin" ./cmd/admin
run_logged 300 "$artifact_root/account-maintain-build.log" \
  go -C "$repo_root/admin" build -o "$runtime_dir/account-maintain" ./cmd/account-maintain
ADMIN_HTTP_ADDR="127.0.0.1:$admin_port" \
ADMIN_PUBLIC_ORIGIN="$web_url" \
ADMIN_ALLOW_LOCAL_HTTP=true \
ADMIN_REGISTER_LIMIT=100 \
MYSQL_HOST=127.0.0.1 \
MYSQL_PORT="$mysql_port" \
MYSQL_DATABASE=rcc \
MYSQL_USER=rcc_admin \
MYSQL_PASSWORD="$mysql_password" \
MYSQL_TLS_MODE=false \
RCC_TIMEOUT_KILL_GRACE_MS=1000 \
  node "$repo_root/scripts/run-with-timeout.cjs" 2400 "$runtime_dir/admin" \
  > "$artifact_root/admin.log" 2>&1 &
admin_pid=$!
wait_for_http Admin "$admin_url/health/ready" "$admin_pid" "$artifact_root/admin.log"

printf 'Starting Web preview on a different dynamic loopback port...\n'
RCC_ADMIN_URL="$admin_url" \
RCC_TIMEOUT_KILL_GRACE_MS=1000 \
  node "$repo_root/scripts/run-with-timeout.cjs" 2400 \
    node "$repo_root/scripts/run-vite-preview.cjs" "$repo_root/web" \
      --host 127.0.0.1 --port "$web_port" --strictPort \
  > "$artifact_root/web.log" 2>&1 &
web_pid=$!
wait_for_http Web "$web_url/platform/query-policies" "$web_pid" "$artifact_root/web.log"

direct_status=$(curl --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' \
  "$admin_url/api/v1/query-policies")
proxy_status=$(curl --silent --show-error --max-time 5 --output /dev/null --write-out '%{http_code}' \
  "$web_url/api/v1/query-policies")
printf 'direct Admin without session: %s\nWeb same-origin proxy without session: %s\n' \
  "$direct_status" "$proxy_status" > "$artifact_root/auth-boundary.txt"
if [[ $direct_status != 401 || $proxy_status != 401 ]]; then
  printf 'authentication boundary failed: direct=%s proxy=%s\n' "$direct_status" "$proxy_status" >&2
  exit 1
fi

RCC_PLAYWRIGHT_MODULE="$repo_root/web/node_modules/playwright" RCC_WEB_URL="$web_url" \
  RCC_ACCOUNT_MAINTAIN="$runtime_dir/account-maintain" \
  MYSQL_HOST=127.0.0.1 MYSQL_PORT="$mysql_port" MYSQL_DATABASE=rcc MYSQL_USER=rcc_admin MYSQL_PASSWORD="$mysql_password" MYSQL_TLS_MODE=false \
  run_timeout 30 node "$repo_root/web/e2e/auth-boundary.cjs" > "$artifact_root/authenticated-boundary.json"

run_browser_suite() {
  local name=$1
  local script=$2
  local output=$3
  local suite_timeout=${4:-180}
  local browser_engine=${5:-${RCC_E2E_ENGINE:-chromium}}
  local status
  mkdir -p "$output"
  printf 'Running %s...\n' "$name"
  if RCC_PLAYWRIGHT_MODULE="$repo_root/web/node_modules/playwright" \
  RCC_E2E_ENGINE="$browser_engine" \
  RCC_E2E_MYSQL_CONTAINER="$mysql_container" RCC_WEB_URL="$web_url" RCC_E2E_ORIGIN="$web_url" RCC_E2E_OUTPUT="$output" RCC_E2E_TABLE=stage1_acceptance_items \
  RCC_ACCOUNT_MAINTAIN="$runtime_dir/account-maintain" MYSQL_HOST=127.0.0.1 MYSQL_PORT="$mysql_port" MYSQL_DATABASE=rcc MYSQL_USER=rcc_admin MYSQL_PASSWORD="$mysql_password" MYSQL_TLS_MODE=false \
    run_timeout "${RCC_E2E_TIMEOUT_SECONDS:-$suite_timeout}" node "$script" \
      > "$output/runner.log" 2>&1; then status=0; else status=$?; fi
  cat "$output/runner.log"
  if [[ $status != 0 && ! -f "$output/result.json" ]]; then
    node -e '
      const fs = require("node:fs");
      const [resultPath, status] = process.argv.slice(1);
      fs.writeFileSync(resultPath, JSON.stringify({
        ok: false,
        runnerExitStatus: Number(status),
        failure: { message: "browser process exited before it could write its own result" },
      }, null, 2) + "\n");
    ' "$output/result.json" "$status"
  fi
  return "$status"
}

if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == unsaved-changes ]]; then
run_browser_suite unsaved-changes "$repo_root/web/e2e/unsaved-changes.cjs" "$artifact_root/unsaved-changes"
fi
if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == rule-clarity ]]; then
run_browser_suite rule-clarity "$repo_root/web/e2e/rule-clarity.cjs" "$artifact_root/rule-clarity"
fi
if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == write-recovery ]]; then
run_browser_suite write-recovery "$repo_root/web/e2e/write-recovery.cjs" "$artifact_root/write-recovery" 360
fi
if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == operation-coverage ]]; then
run_browser_suite operation-coverage "$repo_root/web/e2e/operation-coverage.cjs" "$artifact_root/operation-coverage" 420
fi

if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == complex-fields ]]; then
run_browser_suite complex-fields "$repo_root/web/e2e/complex-fields.cjs" "$artifact_root/complex-fields" 420
fi

if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == browser-accessibility ]]; then
for browser_engine in "${browser_engine_list[@]}"; do
  run_browser_suite "browser-accessibility ($browser_engine)" "$repo_root/web/e2e/browser-accessibility.cjs" "$artifact_root/browser-accessibility/$browser_engine" 420 "$browser_engine"
done
fi

if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == release-workflow ]]; then
run_browser_suite "release drafts" "$repo_root/web/e2e/release-drafts.cjs" "$artifact_root/release-workflow/drafts" 240 chromium
run_browser_suite "release approvals" "$repo_root/web/e2e/release-approvals.cjs" "$artifact_root/release-workflow/approvals" 300 chromium
run_browser_suite "release batches" "$repo_root/web/e2e/release-batches.cjs" "$artifact_root/release-workflow/batches" 600 chromium
for browser_engine in "${browser_engine_list[@]}"; do
  run_browser_suite "publication and rollback ($browser_engine)" "$repo_root/web/e2e/release-rollbacks.cjs" "$artifact_root/release-workflow/rollback-$browser_engine" 420 "$browser_engine"
  run_browser_suite "session, conflict and unknown recovery ($browser_engine)" "$repo_root/web/e2e/accounts.mjs" "$artifact_root/release-workflow/recovery-$browser_engine" 600 "$browser_engine"
done
run_browser_suite "rollback reason history" "$repo_root/web/e2e/release-rollback-reason.cjs" "$artifact_root/release-workflow/rollback-reason" 240 chromium
fi

if [[ ${RCC_E2E_SUITE:-all} == rollback-reason ]]; then
run_browser_suite "rollback reason history" "$repo_root/web/e2e/release-rollback-reason.cjs" "$artifact_root/release-workflow/rollback-reason" 240 chromium
fi

if [[ ${RCC_E2E_SUITE:-all} == all || ${RCC_E2E_SUITE:-all} == field-interactions ]]; then
for browser_engine in "${browser_engine_list[@]}"; do
  run_browser_suite "field interactions ($browser_engine)" "$repo_root/web/e2e/field-interactions.cjs" "$artifact_root/field-interactions/$browser_engine" 420 "$browser_engine"
done
fi

expected='5|1|5|0|notification_page_query_v1|stage1_mutation_v1|1|DEPRECATED'
capture_mysql_state "$artifact_root/database-postcheck.txt"
actual=$(tail -n 1 "$artifact_root/database-postcheck.txt")
if [[ $actual != "$expected" ]]; then
  printf 'database post-check failed\nexpected: %s\nactual:   %s\n' "$expected" "$actual" >&2
  exit 1
fi
capture_fixture_rows "$artifact_root/fixture-after.tsv"
if ! cmp -s "$artifact_root/fixture-before.tsv" "$artifact_root/fixture-after.tsv"; then
  printf 'fixture rows changed during browser acceptance\n' >&2
  exit 1
fi

run_completed=true
printf 'Browser acceptance passed; database post-check: %s\n' "$actual"
