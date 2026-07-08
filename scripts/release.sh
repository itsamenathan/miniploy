#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'USAGE'
Usage: scripts/release.sh vX.Y.Z

Prepares a release by moving CHANGELOG.md's Unreleased notes under the given
version, committing the changelog update, and creating an annotated git tag.

Example:
  scripts/release.sh v0.1.1
USAGE
}

version_arg="${1:-}"
if [[ -z "$version_arg" ]]; then
  usage
  exit 2
fi

if [[ ! "$version_arg" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "version must look like vX.Y.Z, got: $version_arg" >&2
  exit 2
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "working tree must be clean before preparing a release" >&2
  exit 1
fi

version="${version_arg#v}"
date="$(date +%F)"

if grep -q "^## \[$version\]" CHANGELOG.md; then
  echo "CHANGELOG.md already has a section for $version" >&2
  exit 1
fi

python3 - "$version" "$date" <<'PY'
from pathlib import Path
import sys

version, date = sys.argv[1], sys.argv[2]
path = Path("CHANGELOG.md")
text = path.read_text()
marker = "## [Unreleased]"
if marker not in text:
    raise SystemExit("CHANGELOG.md is missing ## [Unreleased]")
text = text.replace(marker, f"{marker}\n\n## [{version}] - {date}", 1)
path.write_text(text)
PY

git add CHANGELOG.md
git commit -m "Prepare $version_arg release"
git tag -a "$version_arg" -m "Release $version_arg"

echo "Prepared $version_arg"
echo "Push with:"
echo "  git push origin main"
echo "  git push origin $version_arg"
