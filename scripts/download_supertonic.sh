#!/bin/bash

# Setup
TARGET_DIR="${HOME}/.local/share/supertonic2"
REPO_URL="https://huggingface.co/Supertone/supertonic-2"

# Create parent directory
mkdir -p "$(dirname "$TARGET_DIR")"

echo "Downloading Supertonic-2 models from Hugging Face to $TARGET_DIR..."

# Clone only if the directory doesn't exist (fast and safe approach)
if [ ! -d "$TARGET_DIR" ]; then
    echo "Cloning repository..."
    # Clone without GIT_LFS_SKIP_SMUDGE=1 to download all large files.
    # Note: git-lfs must be installed (mac: brew install git-lfs)
    git clone $REPO_URL "$TARGET_DIR"
    
    # Optional: clean up .git to save space (if maintaining only the latest files)
    # rm -rf "$TARGET_DIR/.git"
    
    echo "Download completed."
else
    echo "Directory $TARGET_DIR already exists."
    echo "To update the model, navigate to $TARGET_DIR and run 'git pull'."
fi
