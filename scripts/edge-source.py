#!/usr/bin/env python3
"""Select successful Check and CodeQL runs for a trusted main push."""

import json
import os
import urllib.parse
import urllib.request


def latest_success(runs, repository, sha):
    candidates = [run for run in runs if run["head_sha"] == sha and run["event"] in {"push", "workflow_dispatch"}
                  and run["head_branch"] == "main" and run["head_repository"]["full_name"] == repository]
    if not candidates:
        return None
    latest = max(candidates, key=lambda run: run["id"])
    if latest["status"] != "completed" or latest["conclusion"] != "success":
        return None
    return latest


def main():
    if os.environ.get("CONSOLE_EDGE_ENABLED") != "true":
        with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
            output.write("ready=false\n")
        print("Edge publication is disabled.")
        return
    repository, sha = os.environ["GITHUB_REPOSITORY"], os.environ["SOURCE_SHA"]

    def get(path):
        req = urllib.request.Request(f"https://api.github.com/repos/{repository}/{path}", headers={
            "Authorization": "Bearer " + os.environ["GH_TOKEN"], "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2026-03-10", "User-Agent": "console-edge-publication"})
        with urllib.request.urlopen(req) as response:
            return json.load(response)

    query = urllib.parse.urlencode({"head_sha": sha, "branch": "main", "per_page": 100})
    check = latest_success(get("actions/workflows/check.yml/runs?" + query)["workflow_runs"], repository, sha)
    codeql = latest_success(get("actions/workflows/codeql.yml/runs?" + query)["workflow_runs"], repository, sha)
    ready = check is not None and codeql is not None and get("branches/main")["commit"]["sha"] == sha
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        output.write(f"ready={str(ready).lower()}\n")
        if ready:
            output.write(f'run_id={check["id"]}\nrun_attempt={check["run_attempt"]}\n')
    if not ready:
        print("Edge publication waits for successful Check and CodeQL runs on the current main commit.")


if __name__ == "__main__":
    main()
