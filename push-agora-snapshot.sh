#!/usr/bin/env bash
#
# push-agora-snapshot.sh — snapshot the whole full_team_repo polyrepo into the
# single "agora" repo on GitHub, as a flat code dump (no submodules).
#
# Why the .git dance: each team-* service has its OWN nested .git. If we `git add`
# them as-is, Git records them as gitlinks (submodule pointers) and the actual
# FILES never get pushed. So we TEMPORARILY move each nested .git aside, commit the
# real file contents, push, then restore every nested .git. A trap guarantees the
# nested .git dirs are put back even if the commit/push fails or you Ctrl-C.
#
# Your sub-repo histories are never touched or deleted — they're only renamed on
# disk for the few seconds the snapshot commit runs, then restored.
#
# Usage:
#   ./push-agora-snapshot.sh                 # normal push
#   FORCE=1 ./push-agora-snapshot.sh         # force-push (overwrite the snapshot)
#   MSG="my message" ./push-agora-snapshot.sh
#
set -euo pipefail

ROOT="/Users/phuc.buidang/Documents/full_team_repo"
REMOTE_URL="https://github.com/buidangphuc/agora.git"
BRANCH="main"
MSG="${MSG:-snapshot: $(date +%Y-%m-%d) — cluster 25/25 + OpenSpec 15/15 archived}"
DISABLED_SUFFIX="__snapshot_disabled"

cd "$ROOT"

# ---- collect nested .git dirs (every sub-repo, but NOT the root .git) ----
# Portable array fill (works on macOS bash 3.2, no mapfile).
NESTED_GITS=()
while IFS= read -r g; do
  NESTED_GITS+=("$g")
done < <(find . -maxdepth 2 -name .git -type d ! -path './.git' | sort)

restore_nested() {
  for g in "${NESTED_GITS[@]}"; do
    if [ -d "${g}${DISABLED_SUFFIX}" ] && [ ! -e "$g" ]; then
      mv "${g}${DISABLED_SUFFIX}" "$g"
    fi
  done
}
# Always restore nested .git dirs on any exit (success, error, or Ctrl-C).
trap restore_nested EXIT

echo "Found ${#NESTED_GITS[@]} nested sub-repo .git dirs (will be restored after)."

# ---- ensure the root is a git repo pointing at agora ----
if [ ! -d .git ]; then
  git init -q
  git symbolic-ref HEAD "refs/heads/${BRANCH}" 2>/dev/null || git checkout -q -b "${BRANCH}"
fi
git config user.email  "phuc.buidang@batdongsan.com.vn" >/dev/null 2>&1 || true
git config user.name   "buidangphuc" >/dev/null 2>&1 || true
if git remote get-url origin >/dev/null 2>&1; then
  git remote set-url origin "${REMOTE_URL}"
else
  git remote add origin "${REMOTE_URL}"
fi

# ---- move nested .git aside so file contents are captured (not gitlinks) ----
for g in "${NESTED_GITS[@]}"; do
  mv "$g" "${g}${DISABLED_SUFFIX}"
done

# ---- stage everything the root .gitignore allows, commit, push ----
git add -A
if git diff --cached --quiet; then
  echo "Nothing new to commit — repo already matches the snapshot."
else
  git commit -q -m "${MSG}"
  echo "Committed: $(git log -1 --format='%h %s')"
fi

# restore BEFORE pushing so the network step can't strand the sub-repos
restore_nested
trap - EXIT

echo "Pushing to ${REMOTE_URL} (${BRANCH})..."
if [ "${FORCE:-0}" = "1" ]; then
  git push -u origin "${BRANCH}" --force
else
  # A fresh snapshot repo has unrelated history; if a plain push is rejected,
  # re-run with FORCE=1 to overwrite the remote snapshot.
  git push -u origin "${BRANCH}" || {
    echo "Push rejected (likely divergent snapshot history). Re-run with:  FORCE=1 $0"
    exit 1
  }
fi

echo "Done. Snapshot pushed to ${REMOTE_URL}"
echo "Nested sub-repo .git dirs are all restored."
