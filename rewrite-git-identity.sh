#!/bin/bash
# Script to rewrite git history and remove VM hostname from commits
#
# WARNING: This rewrites git history! Only run BEFORE pushing to GitHub.
# After running this, all commit hashes will change.

set -e

RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${YELLOW}Git Identity Rewriter${NC}"
echo "===================="
echo
echo "This script will rewrite ALL git commits to use a new author/committer identity."
echo
echo -e "${RED}WARNING: This changes git history!${NC}"
echo "  - All commit hashes will change"
echo "  - Only safe to do BEFORE pushing to remote"
echo "  - Cannot be easily undone"
echo

if [[ -n "$(git remote)" ]]; then
    echo -e "${YELLOW}Warning: Git remotes detected:${NC}"
    git remote -v
    echo
    echo -e "${RED}If you've already pushed to any of these remotes, DO NOT proceed!${NC}"
    echo "This will cause conflicts and require force-push."
    echo
fi

read -p "Do you want to continue? (yes/no): " confirm
if [[ "$confirm" != "yes" ]]; then
    echo "Aborted."
    exit 1
fi

echo
echo "Enter your desired git identity:"
read -p "Name: " NEW_NAME
read -p "Email: " NEW_EMAIL

if [[ -z "$NEW_NAME" || -z "$NEW_EMAIL" ]]; then
    echo -e "${RED}Error: Name and email are required${NC}"
    exit 1
fi

echo
echo "This will change:"
echo "  Current: $(git log -1 --format='%an <%ae>')"
echo "  New:     $NEW_NAME <$NEW_EMAIL>"
echo
read -p "Confirm? (yes/no): " final_confirm
if [[ "$final_confirm" != "yes" ]]; then
    echo "Aborted."
    exit 1
fi

echo
echo "Creating backup..."
cp -r .git .git.backup
echo -e "${GREEN}✓ Backup created at .git.backup${NC}"

echo
echo "Rewriting git history..."

# Check if git-filter-repo is available
if command -v git-filter-repo &> /dev/null; then
    echo "Using git-filter-repo (recommended)..."
    git filter-repo --force --commit-callback "
        commit.author_name = b'$NEW_NAME'
        commit.author_email = b'$NEW_EMAIL'
        commit.committer_name = b'$NEW_NAME'
        commit.committer_email = b'$NEW_EMAIL'
    "
else
    echo "Using git filter-branch..."
    git filter-branch --force --env-filter "
        export GIT_AUTHOR_NAME='$NEW_NAME'
        export GIT_AUTHOR_EMAIL='$NEW_EMAIL'
        export GIT_COMMITTER_NAME='$NEW_NAME'
        export GIT_COMMITTER_EMAIL='$NEW_EMAIL'
    " --tag-name-filter cat -- --branches --tags
fi

echo -e "${GREEN}✓ Git history rewritten${NC}"

# Set config for future commits
echo
echo "Setting git config for future commits..."
git config user.name "$NEW_NAME"
git config user.email "$NEW_EMAIL"
echo -e "${GREEN}✓ Git config updated${NC}"

echo
echo -e "${GREEN}Done!${NC}"
echo
echo "Verification:"
echo "  New author: $(git log -1 --format='%an <%ae>')"
echo "  Commits: $(git log --oneline | wc -l)"
echo
echo "Next steps:"
echo "  1. Verify commits: git log --format='%an <%ae>' | sort -u"
echo "  2. If everything looks good, delete backup: rm -rf .git.backup"
echo "  3. If you need to undo: rm -rf .git && mv .git.backup .git"
echo
echo "Now safe to push to GitHub!"
