#!/usr/bin/env python3
"""Enforce the Developer Certificate of Origin (DCO) on pull-request commits.

Every non-merge, non-bot commit in the pull request must carry a
``Signed-off-by`` trailer whose name and email match the commit author (the
standard DCO rule, as enforced by the DCO GitHub App).

Commits are read from the GitHub REST API using only the Python standard
library (``urllib`` + ``json``) -- no ``git`` invocation and no third-party
packages. The workflow provides, via the environment:

    GITHUB_TOKEN        token used for the API request
    PR_NUMBER           pull-request number
    GITHUB_REPOSITORY   "owner/repo"            (set automatically by Actions)
    GITHUB_API_URL      API base URL            (set automatically by Actions)

Exit codes: 0 = all commits signed off, 1 = one or more missing, 2 = misconfig.
"""

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.request
from typing import Any

HELP = """\
------------------------------------------------------------------
One or more commits are not signed off for the DCO.

Every commit must carry a trailer matching its author:
  Signed-off-by: Your Name <your.email@example.com>

Fix the most recent commit:
  git commit --amend -s --no-edit

Fix all commits on this branch:
  git rebase --signoff main

Then force-push. See CONTRIBUTING.md for details.
------------------------------------------------------------------\
"""


def is_signed_off(message: str, name: str, email: str) -> bool:
    """True if the commit message has a Signed-off-by trailer for the author."""
    expected = f"signed-off-by: {name} <{email}>".lower()
    return any(line.strip().lower() == expected for line in message.splitlines())


def is_bot(commit: dict[str, Any]) -> bool:
    """True if the commit was authored by a bot (e.g. Dependabot)."""
    author: Any = commit.get("author") or {}
    if author.get("type") == "Bot":
        return True
    commit_author: Any = commit.get("commit", {}).get("author") or {}
    name: str = commit_author.get("name", "")
    return name.endswith("[bot]")


def is_merge(commit: dict[str, Any]) -> bool:
    """True for merge commits (two or more parents), which need no sign-off."""
    return len(commit.get("parents", [])) > 1


def check_commits(commits: list[dict[str, Any]]) -> int:
    """Print a per-commit report and return the number of unsigned commits."""
    failures = 0
    for commit in commits:
        if is_merge(commit) or is_bot(commit):
            continue

        author = commit["commit"]["author"]
        name, email = author["name"], author["email"]
        message = commit["commit"]["message"]
        subject = message.splitlines()[0] if message else ""
        short = commit["sha"][:8]

        if is_signed_off(message, name, email):
            print(f"ok   {short}  {subject}")
        else:
            print(f"FAIL {short}  {subject}")
            print(f"       missing or mismatched: 'Signed-off-by: {name} <{email}>'")
            failures += 1
    return failures


def _next_link(link_header: str) -> str | None:
    """Extract the rel="next" URL from a GitHub Link response header, if any."""
    for part in link_header.split(","):
        segments = part.split(";")
        if len(segments) < 2:
            continue
        url = segments[0].strip().lstrip("<").rstrip(">")
        if any(seg.strip() == 'rel="next"' for seg in segments[1:]):
            return url
    return None


def fetch_commits(api: str, repo: str, pr_number: str, token: str) -> list[dict[str, Any]]:
    """Fetch all commits of a pull request, following pagination."""
    commits: list[dict[str, Any]] = []
    url: str | None = f"{api}/repos/{repo}/pulls/{pr_number}/commits?per_page=100"
    while url:
        request = urllib.request.Request(
            url,
            headers={
                "Authorization": f"Bearer {token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
                "User-Agent": "dco-check",
            },
        )
        with urllib.request.urlopen(request) as response:
            commits.extend(json.load(response))
            url = _next_link(response.headers.get("Link", ""))
    return commits


def main() -> int:
    token = os.environ.get("GITHUB_TOKEN", "").strip()
    pr_number = os.environ.get("PR_NUMBER", "").strip()
    repo = os.environ.get("GITHUB_REPOSITORY", "").strip()
    api = os.environ.get("GITHUB_API_URL", "https://api.github.com").strip()

    if not (token and pr_number and repo):
        print("GITHUB_TOKEN, PR_NUMBER and GITHUB_REPOSITORY must be set", file=sys.stderr)
        return 2

    try:
        commits = fetch_commits(api, repo, pr_number, token)
    except urllib.error.HTTPError as err:
        print(f"GitHub API request failed: {err.code} {err.reason}", file=sys.stderr)
        return 2

    if not commits:
        print("No commits to check.")
        return 0

    # The pull-request commits endpoint returns at most 250 commits.
    if len(commits) == 250:
        print("Warning: PR has 250+ commits; only the first 250 were checked.", file=sys.stderr)

    if check_commits(commits):
        print(HELP, file=sys.stderr)
        return 1

    print("All commits are signed off.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
