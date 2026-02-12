#!/usr/bin/env bash
# Merge a worktree branch into main and cleanup. Usage: ./scripts/worktree-merge.sh 01-org-abstraction

set -e
SPLIT=${1:?Usage: ./scripts/worktree-merge.sh 01-org-abstraction}
REPO_ROOT=$(git rev-parse --show-toplevel)
WORKTREE_DIR="$REPO_ROOT/.worktrees/$SPLIT"
BRANCH="feature/$SPLIT"

cd "$REPO_ROOT"

# Merge
git checkout main
git merge "$BRANCH" -m "Merge $BRANCH: $SPLIT"

# Remove worktree
git worktree remove "$WORKTREE_DIR" --force 2>/dev/null || true
git branch -d "$BRANCH" 2>/dev/null || true

echo "Merged $BRANCH into main and cleaned up worktree"
