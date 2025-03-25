#!/bin/bash
# Script to restructure the CowGnition project to follow standard Go project layout
# Run this from your project root directory (~/projects/cowgnition)

set -e  # Exit on any error

echo "Starting project restructure..."

# 1. Create standard Go project directories
echo "Creating directory structure..."
mkdir -p cmd/server
mkdir -p configs
mkdir -p internal/{auth,config,handler,rtm,server}
mkdir -p pkg/{mcp,rtmapi}
mkdir -p scripts
mkdir -p test/{integration,unit}
mkdir -p assets

# 2. Organize media files
echo "Organizing assets..."
if [ -d "media" ]; then
  if [ -f "media/cowgnition_logo.png" ]; then
    mkdir -p assets
    mv media/cowgnition_logo.png assets/
  fi
  # Only remove media directory if it's empty
  rmdir media 2>/dev/null || echo "Keeping media directory (not empty)"
fi

# 3. Initialize Go module (if it doesn't exist)
echo "Setting up Go module..."
if [ ! -f go.mod ]; then
  go mod init github.com/cowgnition/cowgnition
fi

# 4. Create example config file
echo "Creating config example..."
mkdir -p configs
cat > configs/config.example.yaml << 'EOF'
server:
  name: "CowGnition RTM"
  port: 8080

rtm:
  api_key: "your_api_key"
  shared_secret: "your_shared_secret"

auth:
  token_path: "~/.config/cowgnition/tokens"
EOF

# 5. Create main.go
echo "Creating main application..."
mkdir -p cmd/server
cat > cmd/server/main.go << 'EOF'
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// Version information (populated at build time)
var (
	version = "dev"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/config.yaml", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	// Print version and exit if requested
	if *showVersion {
		fmt.Printf("cowgnition version %s\n", version)
		return
	}

	log.Printf("CowGnition cowgnition Server version %s", version)
	log.Println("This is a placeholder. Implement the server functionality.")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
}
EOF

# 6. Create Makefile
echo "Creating Makefile..."
cat > Makefile << 'EOF'
.PHONY: build clean test

# Variables
BINARY_NAME=cowgnition
MAIN_PACKAGE=./cmd/server
GO_FILES=$(shell find . -name "*.go" -not -path "./vendor/*")
VERSION=$(shell git describe --tags --always --dirty || echo "dev")
LDFLAGS=-ldflags "-X main.version=${VERSION}"

# Default target
all: build

# Build the application
build:
	@echo "Building..."
	go build ${LDFLAGS} -o ${BINARY_NAME} ${MAIN_PACKAGE}

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f ${BINARY_NAME}

# Run with hot reloading (using entr - install with brew install entr)
dev:
	@echo "Starting development server with hot reload..."
	find . -name "*.go" | entr -r go run ${MAIN_PACKAGE} --config configs/config.yaml
EOF

# 7. Create .gitignore
echo "Creating .gitignore..."
cat > .gitignore << 'EOF'
# Binaries for programs and plugins
*.exe
*.exe~
*.dll
*.so
*.dylib
cowgnition

# Test binary, built with `go test -c`
*.test

# Output of the go coverage tool
*.out
*.cov
coverage.html

# Dependency directories
vendor/

# Go workspace file
go.work

# IDE specific files
.idea/
.vscode/
*.swp
*.swo

# System files
.DS_Store
Thumbs.db

# Configuration with potential secrets
configs/config.yaml
!configs/config.example.yaml
.env
*_token*
EOF

# 8. Create setup script
echo "Creating setup script..."
mkdir -p scripts
cat > scripts/setup.sh << 'EOF'
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
EOF
chmod +x scripts/setup.sh

# 9. Add initial Go dependencies
echo "Installing dependencies..."
go mod tidy || echo "You may need to run 'go get' commands manually"

echo "Project restructuring complete!"
echo "Next steps:"
echo "1. Run './scripts/setup.sh' to set up your development environment"
echo "2. Create a config file: cp configs/config.example.yaml configs/config.yaml"
echo "3. Edit the config file with your RTM API credentials"
echo "4. Build the project: make build"
echo "5. Run it: ./cowgnition --config configs/config.yaml"
