#!/bin/bash
# Setup script for development environment

# Create config from example if it doesn't exist
if [ ! -f configs/config.yaml ]; then
  echo "Creating config.yaml from example..."
  cp configs/config.example.yaml configs/config.yaml
  echo "Please edit configs/config.yaml to add your RTM API credentials"
fi

# Create token directory
mkdir -p ~/.config/cowgnition/tokens

# Install development tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install golang.org/x/tools/cmd/goimports@latest
go install golang.org/x/tools/cmd/godoc@latest

# Install hot reload tool
if command -v brew &> /dev/null; then
  brew install entr
else
  echo "Please install 'entr' manually for hot reloading functionality"
fi

echo "Setup complete! You can now build the project with 'make build'"
