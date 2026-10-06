#!/usr/bin/env python3
"""Prepare private files for an isolated Keycloak development container."""

import json
import os
import secrets
import subprocess
import uuid
from pathlib import Path


def main() -> None:
    directory = Path(__file__).resolve().parent.parent / '.local' / 'keycloak'
    directory.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(directory, 0o700)
    if any(directory.iterdir()):
        raise SystemExit('Development files already exist. Preserve them or choose a new isolated setup.')
    settings = {
        'issuer': 'https://localhost:8543/realms/console-dev',
        'clientId': 'openbao-console',
        'clientSecret': secrets.token_urlsafe(32),
        'subject': str(uuid.uuid4()),
        'username': 'platform-engineer',
        'password': secrets.token_urlsafe(24),
    }
    realm = {
        'realm': 'console-dev', 'enabled': True, 'sslRequired': 'all',
        'registrationAllowed': False,
        'clients': [{
            'clientId': settings['clientId'], 'protocol': 'openid-connect',
            'publicClient': False, 'secret': settings['clientSecret'],
            'standardFlowEnabled': True, 'implicitFlowEnabled': False,
            'directAccessGrantsEnabled': False, 'serviceAccountsEnabled': False,
            'redirectUris': ['https://127.0.0.1:8554/auth/callback', 'https://localhost:8554/auth/callback'],
            'defaultClientScopes': ['profile', 'email'],
            'attributes': {'pkce.code.challenge.method': 'S256'},
        }],
        'users': [{
            'id': settings['subject'], 'username': settings['username'], 'enabled': True,
            'firstName': 'Platform', 'lastName': 'Engineer', 'email': 'engineer@example.invalid',
            'emailVerified': True,
            'credentials': [{'type': 'password', 'value': settings['password'], 'temporary': False}],
        }],
    }
    configuration = '''[req]
distinguished_name = subject
x509_extensions = extensions
prompt = no
[subject]
CN = Console development
[extensions]
subjectAltName = DNS:localhost,IP:127.0.0.1
basicConstraints = critical,CA:TRUE,pathlen:0
keyUsage = critical,digitalSignature,keyEncipherment,keyCertSign
extendedKeyUsage = serverAuth
'''
    (directory / 'openssl.cnf').write_text(configuration)
    result = subprocess.run([
        'openssl', 'req', '-x509', '-newkey', 'rsa:2048', '-sha256', '-nodes', '-days', '7',
        '-config', str(directory / 'openssl.cnf'), '-keyout', str(directory / 'server.key'),
        '-out', str(directory / 'server.crt'),
    ], capture_output=True, check=False)
    if result.returncode != 0:
        raise SystemExit('Development certificate generation failed.')
    (directory / 'credentials.json').write_text(json.dumps(settings, indent=2) + '\n')
    (directory / 'realm.json').write_text(json.dumps(realm, indent=2) + '\n')
    (directory / 'compose.env').write_text(f'KEYCLOAK_DEV_GROUP_ID={os.getgid()}\n')
    for path in directory.iterdir():
        os.chmod(path, 0o640 if path.name in {'realm.json', 'server.key', 'server.crt'} else 0o600)
    print('Created private development files in .local/keycloak. No system trust was changed.')


if __name__ == '__main__':
    main()
