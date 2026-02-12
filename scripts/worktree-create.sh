#!/usr/bin/env bash
# Create worktree for a split. Usage: ./scripts/worktree-create.sh 01-org-abstraction

set -e
SPLIT=${1:?Usage: ./scripts/worktree-create.sh 01-org-abstraction}
REPO_ROOT=$(git rev-parse --show-toplevel)
WORKTREE_DIR="$REPO_ROOT/.worktrees/$SPLIT"
BRANCH="feature/$SPLIT"

cd "$REPO_ROOT"

# Ensure .worktrees in gitignore
if ! grep -q '^\.worktrees/$' .gitignore 2>/dev/null; then
  echo ".worktrees/" >> .gitignore
  git add .gitignore
  git commit -m "chore: add .worktrees to gitignore" || true
fi

# Create worktree from main
git worktree add "$WORKTREE_DIR" -b "$BRANCH" main

echo "Worktree ready at $WORKTREE_DIR"
echo "  cd $WORKTREE_DIR"
echo "  /deep-plan @planning/$SPLIT/spec.md"
