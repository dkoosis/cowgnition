#!/bin/bash
# Setup script for development environment

set -e

# Colors
GREEN="\033[0;32m"
BLUE="\033[0;34m"
YELLOW="\033[0;33m"
NC="\033[0m" # No Color

echo -e "${BLUE}Setting up CowGnition development environment...${NC}"

# Create config from example if it doesn't exist
if [ ! -f configs/config.yaml ]; then
  echo -e "${YELLOW}Creating config.yaml from example...${NC}"
  cp configs/config.example.yaml configs/config.yaml
  echo -e "${YELLOW}Please edit configs/config.yaml to add your RTM API credentials${NC}"
fi

# Create token directory
echo -e "${YELLOW}Creating token directory...${NC}"
mkdir -p ~/.config/cowgnition/tokens

# Install development tools
echo -e "${YELLOW}Installing development tools...${NC}"
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/tools/cmd/godoc@latest
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install gotest.tools/gotestsum@latest

# Install hot reload tool
if command -v brew &> /dev/null; then
  echo -e "${YELLOW}Installing entr for hot reloading...${NC}"
  brew install entr
else
  echo -e "${YELLOW}Please install 'entr' manually for hot reloading functionality${NC}"
fi

# Set up git hooks
echo -e "${YELLOW}Setting up Git hooks...${NC}"
mkdir -p .git/hooks
if [ -f ".git/hooks/pre-commit" ]; then
  echo -e "${YELLOW}Backing up existing pre-commit hook to pre-commit.bak${NC}"
  mv .git/hooks/pre-commit .git/hooks/pre-commit.bak
fi
cp scripts/pre-commit .git/hooks/
chmod +x .git/hooks/pre-commit

# Create or update .golangci.yml