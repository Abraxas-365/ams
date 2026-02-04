#!/bin/bash

# Usage: ./init-project.sh <new-module-name> [target-directory]
# Example: ./init-project.sh github.com/myuser/myproject my-project

set -e

OLD_MODULE="github.com/Abraxas-365/manifesto"
REPO_URL="https://github.com/Abraxas-365/manifesto.git"

# Check arguments
if [ -z "$1" ]; then
    echo "Usage: $0 <new-module-name> [target-directory]"
    echo "Example: $0 github.com/myuser/myproject my-project"
    exit 1
fi

NEW_MODULE="$1"
TARGET_DIR="${2:-$(basename $NEW_MODULE)}"

echo "🚀 Initializing new project from manifesto skeleton..."
echo "   New module: $NEW_MODULE"
echo "   Target directory: $TARGET_DIR"
echo ""

# Clone the repository
echo "📦 Cloning skeleton repository..."
git clone "$REPO_URL" "$TARGET_DIR"

# Navigate to the new directory
cd "$TARGET_DIR"

# Remove git history
echo "🗑️  Removing old git history..."
rm -rf .git

# Replace module name in go.mod
echo "📝 Updating go.mod..."
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    sed -i '' "s|$OLD_MODULE|$NEW_MODULE|g" go.mod
else
    # Linux
    sed -i "s|$OLD_MODULE|$NEW_MODULE|g" go.mod
fi

# Replace module name in all .go files
echo "🔄 Updating import statements in .go files..."
find . -type f -name "*.go" -print0 | while IFS= read -r -d '' file; do
    if [[ "$OSTYPE" == "darwin"* ]]; then
        sed -i '' "s|$OLD_MODULE|$NEW_MODULE|g" "$file"
    else
        sed -i "s|$OLD_MODULE|$NEW_MODULE|g" "$file"
    fi
done

# Initialize new git repository
echo "🎉 Initializing new git repository..."
git init
git add .
git commit -m "Initial commit from manifesto skeleton"

# Run go mod tidy to clean up dependencies
echo "🧹 Running go mod tidy..."
go mod tidy

echo ""
echo "✅ Project initialized successfully!"
echo "   Location: $TARGET_DIR"
echo "   Module: $NEW_MODULE"
echo ""
echo "Next steps:"
echo "   cd $TARGET_DIR"
echo "   git remote add origin <your-repo-url>"
echo "   git push -u origin main"

