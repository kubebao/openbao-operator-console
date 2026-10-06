#!/usr/bin/env python3
"""Create a scoped, short-lived reader kubeconfig through an administrator context."""

import argparse
import json
import os
import subprocess
import tempfile
from pathlib import Path
from urllib.parse import urlparse


def kubectl(prefix: list[str], arguments: list[str]) -> str:
    result = subprocess.run(prefix + arguments, capture_output=True, text=True, check=False)
    if result.returncode != 0:
        raise SystemExit("Kubernetes credential request failed. Check the administrator context and reader RBAC.")
    return result.stdout


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--admin-kubeconfig", required=True)
    parser.add_argument("--context", required=True)
    parser.add_argument("--namespace", default="console-system")
    parser.add_argument("--service-account", default="console-reader")
    parser.add_argument("--duration", default="1h")
    parser.add_argument("--output", required=True)
    arguments = parser.parse_args()
    output = Path(arguments.output).expanduser()
    if output.resolve() == Path(arguments.admin_kubeconfig).expanduser().resolve():
        raise SystemExit("The reader output must differ from the administrator kubeconfig.")
    prefix = ["kubectl", "--kubeconfig", arguments.admin_kubeconfig, "--context", arguments.context]
    configuration = json.loads(kubectl(prefix, ["config", "view", "--minify", "--flatten", "--raw", "-o", "json"]))
    cluster = configuration["clusters"][0]
    if urlparse(cluster["cluster"]["server"]).scheme != "https" or cluster["cluster"].get("insecure-skip-tls-verify"):
        raise SystemExit("The reader requires an HTTPS Kubernetes endpoint with certificate verification.")
    token = kubectl(prefix, ["create", "token", arguments.service_account, "--namespace", arguments.namespace, "--duration", arguments.duration]).strip()
    reader = {
        "apiVersion": "v1", "kind": "Config", "clusters": [cluster],
        "contexts": [{"name": arguments.context, "context": {"cluster": cluster["name"], "user": "console-reader"}}],
        "current-context": arguments.context, "users": [{"name": "console-reader", "user": {"token": token}}],
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=output.parent, delete=False) as temporary:
        temporary_path = Path(temporary.name)
        try:
            os.chmod(temporary_path, 0o600)
            json.dump(reader, temporary, indent=2)
            temporary.write("\n")
            temporary.flush()
            os.fsync(temporary.fileno())
            os.replace(temporary_path, output)
        finally:
            temporary_path.unlink(missing_ok=True)
    print("Wrote reader kubeconfig with mode 0600. Refresh it before the token expires.")


if __name__ == "__main__":
    main()
