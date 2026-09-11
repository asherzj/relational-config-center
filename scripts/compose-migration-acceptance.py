#!/usr/bin/env python3
"""Exercise the documented Compose deployment against one disposable MySQL volume.

Never accepts a user's project or database: the generated project name owns every
container/volume it creates and is the sole target of cleanup.
"""
import argparse
import datetime
import json
import http.client
import os
import re
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--artifacts', type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    artifacts = args.artifacts or Path(tempfile.mkdtemp(prefix='rcc-compose-migrations-'))
    artifacts.mkdir(parents=True, exist_ok=True)
    project = 'rcc-goose-' + secrets.token_hex(6)
    env = dict(os.environ, MYSQL_ROOT_PASSWORD=secrets.token_hex(16), MYSQL_PASSWORD=secrets.token_hex(16),
               COMPOSE_PARALLEL_LIMIT='1', MYSQL_DATABASE='rcc', MYSQL_USER='rcc_admin', MYSQL_PUBLISHED_PORT='0', ADMIN_PUBLISHED_PORT='0',
               ADMIN_PUBLIC_ORIGIN='http://127.0.0.1:5173', ADMIN_ALLOW_LOCAL_HTTP='true')
    compose = ['docker', 'compose', '--project-name', project, '--file', str(root / 'deploy/docker-compose.yml')]
    report = {'project': project, 'cases': []}
    log = (artifacts / 'compose.log').open('w')

    def run(arguments, *, sql=None, success=True, timeout=180):
        result = subprocess.run(compose + arguments, env=env, input=sql, text=True,
                                stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout)
        log.write('$ docker compose ' + ' '.join(arguments) + '\n' + result.stdout + '\n')
        log.flush()
        if success and result.returncode:
            raise RuntimeError(f'Compose {arguments} exited {result.returncode}; see {artifacts / "compose.log"}')
        return result

    def sql(statement):
        return run(['exec', '-T', 'mysql', 'sh', '-ec',
                    'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql --default-character-set=utf8mb4 --batch --skip-column-names -uroot "$MYSQL_DATABASE"'], sql=statement).stdout.strip()

    def mysql_ready():
        run(['up', '-d', '--wait', 'mysql'], timeout=120)

    def services():
        ids = run(['ps', '--all', '--quiet']).stdout.split()
        if not ids:
            return {}
        result = subprocess.run(['docker', 'inspect', *ids], env=env, check=True, capture_output=True, text=True, timeout=30)
        return {item['Config']['Labels']['com.docker.compose.service']: item['State'] for item in json.loads(result.stdout)}

    def ready():
        endpoint = run(['port', 'admin', '8080']).stdout.strip()
        url = 'http://' + endpoint + '/health/ready'
        # This generated endpoint is always loopback; never route it through a user proxy.
        client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            try:
                with client.open(url, timeout=2) as response:
                    if response.status == 200 and json.load(response) == {'status': 'ready'}:
                        return
            except (urllib.error.URLError, http.client.HTTPException, OSError):
                pass
            time.sleep(.2)
        raise AssertionError('Admin did not become ready')

    def timepoint(value):
        # Docker timestamps carry nanoseconds; datetime accepts microseconds.
        return datetime.datetime.fromisoformat(re.sub(r'(\.\d{6})\d+', r'\1', value).replace('Z', '+00:00'))

    def verify_success(name):
        run(['up', '-d'], timeout=180)
        ready()
        state = services()
        migration, fixture, admin = [state[key] for key in ('schema-migrate', 'mysql-local-fixture', 'admin')]
        assert migration['Status'] == fixture['Status'] == 'exited'
        assert migration['ExitCode'] == fixture['ExitCode'] == 0
        assert timepoint(migration['FinishedAt']) <= timepoint(fixture['StartedAt'])
        assert timepoint(fixture['FinishedAt']) <= timepoint(admin['StartedAt'])
        assert sql('SELECT COUNT(*) FROM notification_templates;') == '3'
        report['cases'].append({'case': name, 'result': 'PASS', 'services': state})
        print(name + ': PASS', flush=True)

    def recreate_containers():
        # Removes only this generated project's containers, retaining its volume.
        run(['down'], timeout=90)

    def preserved():
        return sql("SELECT id,note FROM business_marker ORDER BY id; SELECT id,roles,role_version,session_version FROM rcc_accounts ORDER BY id; SELECT id,template_key,body FROM notification_templates ORDER BY id; SELECT version_id,is_applied FROM rcc_goose_db_version ORDER BY id;")

    def verify_blocked(name, expected):
        result = run(['up', '-d'], success=False, timeout=180)
        assert result.returncode != 0, name + ' did not fail deployment'
        state = services()
        assert state['schema-migrate']['ExitCode'] != 0
        for service in ('mysql-local-fixture', 'admin'):
            assert service not in state or state[service]['Status'] == 'created', f'{service} started after migration failure'
        assert expected in run(['logs', 'schema-migrate']).stdout
        report['cases'].append({'case': name, 'result': 'PASS', 'services': state})
        print(name + ': PASS', flush=True)

    try:
        run(['build'], timeout=900)
        verify_success('fresh_volume')
        sql("CREATE TABLE business_marker(id int PRIMARY KEY,note text); INSERT INTO business_marker VALUES(17,'preserved through deployment'); INSERT INTO rcc_accounts(id,username,email,display_name,password_hash,roles,role_version,session_version,created_at) VALUES('compose-account','compose.account','compose@example.test','Retained','fixture-hash',31,4,7,'2025-01-02');")
        before = preserved()
        recreate_containers()
        verify_success('adopted_volume_repeat')
        assert preserved() == before, 'repeat deployment changed account/business/version/fixture data'

        # A second, fresh isolated volume represents a genuine pre-Goose install.
        run(['down', '--volumes'], timeout=90)
        mysql_ready()
        sql((root / 'admin/cmd/admin/testdata/pre-goose-8b5cd859.sql').read_text())
        for migration in ['014-table-field-policies.sql', '015-original-order-executions.sql', '016-draft-target-reservations.sql', '017-release-main-order.sql']:
            sql((root / 'deploy/mysql/migrations' / migration).read_text())
        sql("CREATE TABLE business_marker(id int PRIMARY KEY,note text); INSERT INTO business_marker VALUES(17,'unmanaged retained');")
        verify_blocked('unmanaged_volume_blocks', 'baseline_required')
        assert sql("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('notification_templates','rcc_goose_db_version','rcc_schema_migration_attempts');") == '0'
        assert sql('SELECT note FROM business_marker WHERE id=17;') == 'unmanaged retained'
        run(['run', '--rm', 'schema-migrate', 'baseline'])
        verify_success('explicit_adoption_restores_deployment')
        before = preserved()
        # Baseline and forward upgrades are separate durable attempts. Interrupt
        # only the latest successful up; earlier confirmed history must stay intact.
        target_attempt = int(sql("SELECT id FROM rcc_schema_migration_attempts WHERE state='SUCCEEDED' ORDER BY id DESC LIMIT 1;"))
        other_attempts = sql(f"SELECT * FROM rcc_schema_migration_attempts WHERE id<>{target_attempt} ORDER BY id;")
        sql(f"UPDATE rcc_schema_migration_attempts SET state='RUNNING',finished_at=NULL WHERE id={target_attempt} AND state='SUCCEEDED';")
        assert sql("SELECT COUNT(*) FROM rcc_schema_migration_attempts WHERE state IN ('RUNNING','BASELINING');") == '1'
        assert sql(f"SELECT * FROM rcc_schema_migration_attempts WHERE id<>{target_attempt} ORDER BY id;") == other_attempts
        recreate_containers()
        verify_blocked('unconfirmed_volume_blocks', 'recovery_required')
        assert preserved() == before, 'blocked deployment changed existing data'
        run(['run', '--rm', 'schema-migrate', 'recover'])
        verify_success('explicit_recovery_restores_deployment')
        assert preserved() == before, 'recovery deployment changed existing data'
        assert sql(f"SELECT * FROM rcc_schema_migration_attempts WHERE id<>{target_attempt} ORDER BY id;") == other_attempts, 'recovery changed another migration attempt'
    finally:
        run(['logs', '--no-color'], success=False)
        run(['down', '--volumes', '--remove-orphans'], success=False, timeout=120)
        log.close()
        (artifacts / 'results.json').write_text(json.dumps(report, indent=2))
        print('Evidence: ' + str(artifacts), flush=True)


if __name__ == '__main__':
    main()
