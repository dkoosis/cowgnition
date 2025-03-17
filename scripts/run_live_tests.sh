#!/bin/bash
# Run RTM live tests with appropriate environment variables and flags

# Colors for output
GREEN="\033[0;32m"
YELLOW="\033[0;33m"
RED="\033[0;31m"
BLUE="\033[0;34m"
NC="\033[0m" # No Color

# Default values
INTERACTIVE=false
DEBUG=false
CONFIG_PATH=""
TEST_PATTERN="Live"

# Parse arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    -i|--interactive)
      INTERACTIVE=true
      shift
      ;;
    -d|--debug)
      DEBUG=true
      shift
      ;;
    -c|--config)
      CONFIG_PATH="$2"
      shift 2
      ;;
    -p|--pattern)
      TEST_PATTERN="$2"
      shift 2
      ;;
    -s|--skip)
      SKIP_TESTS=true
      shift
      ;;
    -h|--help)
      echo "Usage: $0 [options]"
      echo "Options:"
      echo "  -i, --interactive  Enable interactive authentication prompt"
      echo "  -d, --debug        Enable debug logging for RTM API calls"
      echo "  -c, --config PATH  Path to test configuration file"
      echo "  -p, --pattern PAT  Test pattern to run (default: Live)"
      echo "  -s, --skip         Skip live tests (useful to verify without running)"
      echo "  -h, --help         Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use -h or --help for usage information"
      exit 1
      ;;
  esac
done

# Check if config file exists if provided
if [ -n "$CONFIG_PATH" ] && [ ! -f "$CONFIG_PATH" ]; then
  echo -e "${YELLOW}Warning: Config file not found: $CONFIG_PATH${NC}"
  
  # Create empty config if the user wants to proceed
  read -p "Create empty config file? (y/n) " -n 1 -r
  echo
  if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}Creating empty config file: $CONFIG_PATH${NC}"
    mkdir -p "$(dirname "$CONFIG_PATH")"
    echo '{
  "rtm": {
    "api_key": "",
    "shared_secret": ""
  },
  "options": {
    "skip_live_tests": false,
    "debug_mode": false,
    "max_api_requests": 100
  }
}' > "$CONFIG_PATH"
    echo -e "${GREEN}Config file created. Please edit it to add your RTM API credentials.${NC}"
    exit 0
  fi
fi

# Set environment variables
if [ "$INTERACTIVE" = true ]; then
  export RTM_INTERACTIVE=true
fi

if [ "$DEBUG" = true ]; then
  export RTM_TEST_DEBUG=true
fi

if [ "$SKIP_TESTS" = true ]; then
  export RTM_SKIP_LIVE_TESTS=true
fi

if [ -n "$CONFIG_PATH" ]; then
  export RTM_CONFIG_PATH="$CONFIG_PATH"
fi

# Print configuration
echo -e "${BLUE}Running live tests with:${NC}"
echo -e "  Interactive:  ${YELLOW}$INTERACTIVE${NC}"
echo -e "  Debug mode:   ${YELLOW}$DEBUG${NC}"
echo -e "  Skip tests:   ${YELLOW}$SKIP_TESTS${NC}"
echo -e "  Config path:  ${YELLOW}$CONFIG_PATH${NC}"
echo -e "  Test pattern: ${YELLOW}$TEST_PATTERN${NC}"
echo

# Run the tests
TEST_COMMAND="go test -v ./test/conformance/... -run $TEST_PATTERN"
echo -e "${BLUE}Running: $TEST_COMMAND${NC}"
echo

# Execute the test command
if eval "$TEST_COMMAND"; then
  echo -e "${GREEN}All tests passed!${NC}"
  exit 0
else
  echo -e "${RED}Some tests failed.${NC}"
  exit 1
fi
