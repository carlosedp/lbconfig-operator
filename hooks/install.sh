#!/bin/bash
# Install Git hooks from the hooks directory

HOOKS_DIR="$(cd "$(dirname "$0")" && pwd)"
GIT_HOOKS_DIR="$(git rev-parse --git-dir)/hooks"

# Check if hooks are already installed (via symlink)
if [ -L "${GIT_HOOKS_DIR}/pre-push" ] && [ "$(readlink -f "${GIT_HOOKS_DIR}/pre-push")" = "$(readlink -f "${HOOKS_DIR}/pre-push")" ]; then
    exit 0  # Already installed, exit silently
fi

echo "Installing Git hooks from ${HOOKS_DIR} to ${GIT_HOOKS_DIR}..."

for hook in "${HOOKS_DIR}"/*; do
    # Skip the install script itself and any backup files
    if [[ "$(basename "$hook")" == "install.sh" ]] || [[ "$(basename "$hook")" == *.sample ]]; then
        continue
    fi
    
    hook_name=$(basename "$hook")
    target="${GIT_HOOKS_DIR}/${hook_name}"
    
    # Create symlink
    if [ -e "$target" ]; then
        echo "  Backing up existing ${hook_name} to ${hook_name}.backup"
        mv "$target" "${target}.backup"
    fi
    
    ln -sf "../../hooks/${hook_name}" "$target"
    chmod +x "$hook"
    echo "  ✓ Installed ${hook_name}"
done

echo "✅ Git hooks installed successfully!"
echo ""
echo "To bypass hooks temporarily, use: git push --no-verify"
