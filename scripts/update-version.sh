#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
tap_repo="$repo_root/tap"
cd "$repo_root"

cli_file="$repo_root/internal/cli/cli.go"
formula_file_rel="Formula/morningweave.rb"
formula_file="$tap_repo/$formula_file_rel"

die() {
  echo "error: $*" >&2
  exit 1
}

info() {
  echo "==> $*"
}

current_cli_version="$(awk -F'"' '/^var Version =/ {print $2; exit}' "$cli_file" 2>/dev/null || true)"
current_formula_version="$(awk -F'"' '/^  version / {print $2; exit}' "$formula_file" 2>/dev/null || true)"

info "Current CLI version: ${current_cli_version:-unknown}"
info "Current formula version: ${current_formula_version:-unknown}"

read -r -p "New version (e.g. 1.0.1): " new_version_raw
new_version="${new_version_raw#v}"
if [[ -z "$new_version" ]]; then
  die "version is required"
fi

if [[ -n "$(git status --porcelain)" ]]; then
  die "working tree is dirty; commit or stash before running"
fi
if [[ -n "$(git -C "$tap_repo" status --porcelain)" ]]; then
  die "tap repo is dirty; commit or stash before running"
fi

branch="$(git rev-parse --abbrev-ref HEAD)"
tap_branch="$(git -C "$tap_repo" rev-parse --abbrev-ref HEAD)"
tag="v${new_version}"

info "Bumping CLI version to ${new_version}"
perl -0pi -e "s/^var Version = \".*\"/var Version = \"${new_version}\"/m" "$cli_file"

if git diff --quiet -- "$cli_file"; then
  info "CLI version already set to ${new_version}"
else
  git add "$cli_file"
  git commit -m "Bump version to ${tag}"
fi

info "Creating and pushing tag ${tag}"
git tag -a "$tag" -m "$tag"
git push origin "$branch"
git push origin "$tag"

tarball_url="https://github.com/mbtz/morningweave/archive/refs/tags/${tag}.tar.gz"
info "Computing SHA256 for ${tarball_url}"
sha="$(curl -L "$tarball_url" | shasum -a 256 | awk '{print $1}')"
if [[ -z "$sha" ]]; then
  die "failed to compute sha256"
fi

info "Updating Homebrew formula to ${new_version}"
perl -0pi -e "s|^  url \".*\"|  url \"${tarball_url}\"|m" "$formula_file"
perl -0pi -e "s/^  sha256 \".*\"/  sha256 \"${sha}\"/m" "$formula_file"
perl -0pi -e "s/^  version \".*\"/  version \"${new_version}\"/m" "$formula_file"

git -C "$tap_repo" add "$formula_file_rel"
git -C "$tap_repo" commit -m "Update Homebrew formula for ${tag}"
git -C "$tap_repo" push origin "$tap_branch"

info "Done. Updated formula sha256: ${sha}"
