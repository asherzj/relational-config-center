#!/usr/bin/env python3
"""One authenticated script session; credentials stay in memory, never argv."""
import argparse
import getpass
import http.cookiejar
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--username', required=True)
    parser.add_argument('--password-stdin', action='store_true')
    parser.add_argument('--table', help='Optionally query one enabled Managed Table')
    args = parser.parse_args()
    origin = args.origin.rstrip('/')
    parsed = urllib.parse.urlsplit(origin)
    if (parsed.scheme not in ('http', 'https') or not parsed.hostname
            or parsed.path or parsed.query or parsed.fragment or parsed.username
            or parsed.password or (parsed.scheme == 'http'
                                   and parsed.hostname not in ('127.0.0.1', '::1', 'localhost'))):
        parser.error('origin must be an exact HTTPS origin or explicit loopback HTTP origin')
    password = sys.stdin.read() if args.password_stdin else getpass.getpass('Password: ')
    # CookieJar and CSRF are deliberately memory-only. Never enable HTTP debug logs.
    jar = http.cookiejar.CookieJar()
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, req, fp, code, msg, headers, newurl):
            return None
    client = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar), NoRedirect())
    csrf = ''

    def request(method, path, body=None):
        headers = {'Origin': origin, 'Content-Type': 'application/json'}
        if method != 'GET':
            headers['X-CSRF-Token'] = csrf
        payload = json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(origin + path, payload, headers, method=method)
        with client.open(req, timeout=10) as response:
            data = response.read()
            return json.loads(data) if data else None

    authenticated = False
    try:
        csrf = request('GET', '/api/v1/auth/csrf')['csrf_token']
        identity = request('POST', '/api/v1/auth/login', {'username': args.username, 'password': password})
        password = ''
        authenticated = True
        csrf = identity['csrf_token']
        print('Login succeeded.')
        request('GET', '/api/v1/table-policies')
        # Report activity only while doing actual foreground work. Background
        # polling must not keep an otherwise idle session alive.
        request('POST', '/api/v1/auth/activity')
        if args.table:
            result = request('POST', '/api/v1/tables/' + urllib.parse.quote(args.table, safe='') + '/query',
                             {'page_number': 1, 'page_size': 20})
            print(f'Query succeeded: {len(result["rows"])} rows (values omitted).')
    except urllib.error.HTTPError as error:
        print(f'HTTP {error.code}; no request was automatically replayed.', file=sys.stderr)
        return 1
    except (urllib.error.URLError, TimeoutError, ValueError, KeyError):
        print('Response unavailable or invalid; verify the outcome before retrying.', file=sys.stderr)
        return 1
    finally:
        if authenticated:
            try:
                request('POST', '/api/v1/auth/logout')
                print('Session logged out.')
            except (urllib.error.URLError, TimeoutError, ValueError):
                print('Logout result unconfirmed; this script did not retry.', file=sys.stderr)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
