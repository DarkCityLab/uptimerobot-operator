#!/usr/bin/env bash
# Cut a release by bumping every version-bearing source file in lockstep.
#
# The published container image and Helm chart are versioned from the git tag by
# .github/workflows/release.yml (image tag = ${tag#v}; chart --version/--app-version
# = same), and the chart deploys image tag `.Chart.Version` by default. So image,
# chart, and the image the chart deploys can never drift from the tag. This script
# keeps the *source* files (Chart.yaml, README install commands, release-please
# manifest) matching that tag — the one thing the tag doesn't control. The `guard`
# job in release.yml re-checks this and fails the release if anything is off.
#
# Usage:
#   hack/release.sh 0.1.3           # bump + commit + tag (prints the push command)
#   hack/release.sh 0.1.3 --push    # bump + commit + tag + push main and the tag
set -euo pipefail

V="${1:-}"
PUSH="${2:-}"
if [[ ! "$V" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 X.Y.Z [--push]" >&2
  exit 2
fi

cd "$(git rev-parse --show-toplevel)"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree is dirty — commit or stash first so the release commit is just the bump" >&2
  exit 1
fi

# Bump every version-bearing file (perl -i is identical on macOS and Linux).
perl -pi -e "s/^version: .*/version: $V # x-release-please-version/"   dist/chart/Chart.yaml
perl -pi -e "s/^appVersion: .*/appVersion: \"$V\"/"                     dist/chart/Chart.yaml
perl -pi -e "s/VERSION=[0-9]+\.[0-9]+\.[0-9]+/VERSION=$V/g"             README.md
perl -pi -e "s/(\"\\.\"\s*:\s*)\"[0-9]+\.[0-9]+\.[0-9]+\"/\${1}\"$V\"/" .release-please-manifest.json
# Local kind test image tag (functional only — not a published artifact).
perl -pi -e "s/uptimerobot-operator:v[0-9]+\.[0-9]+\.[0-9]+/uptimerobot-operator:v$V/g" \
  .github/workflows/test-chart.yml

# Self-check: no stray mismatched release version remains in the synced files.
drift=0
chk() { if [[ "$2" != "$V" ]]; then echo "drift: $1 = '$2' (expected $V)" >&2; drift=1; fi; }
chk "Chart.yaml version"      "$(grep -E '^version:'    dist/chart/Chart.yaml | awk '{print $2}')"
chk "Chart.yaml appVersion"   "$(grep -E '^appVersion:' dist/chart/Chart.yaml | tr -d '"' | awk '{print $2}')"
chk "release-please manifest" "$(grep -oE '[0-9]+\.[0-9]+\.[0-9]+' .release-please-manifest.json | head -1)"
while IFS= read -r v; do chk "README VERSION=" "$v"; done \
  < <(grep -oE 'VERSION=[0-9]+\.[0-9]+\.[0-9]+' README.md | cut -d= -f2)
[[ "$drift" -eq 0 ]] || { echo "aborting: bump left a mismatch" >&2; exit 1; }

git add -A
git -c user.name=ogizhelev -c user.email=ogizhelev@users.noreply.github.com \
  commit -m "chore: release $V" >/dev/null
git tag "v$V"
echo "Committed 'chore: release $V' and tagged v$V."

if [[ "$PUSH" == "--push" ]]; then
  git push origin HEAD
  git push origin "v$V"
  echo "Pushed main and v$V — release.yml will build, sign, and publish the image + chart."
else
  echo "Push to publish:  git push origin HEAD && git push origin v$V"
fi
