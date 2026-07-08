# Extract a Keep a Changelog section by version.
# Usage: awk -v version=0.1.0 -f scripts/changelog-section.awk CHANGELOG.md

BEGIN {
  target = "## [" version "]"
  in_section = 0
}

$0 ~ /^## \[/ {
  if (in_section) {
    exit
  }
  if (index($0, target) == 1) {
    in_section = 1
    next
  }
}

in_section {
  print
}
