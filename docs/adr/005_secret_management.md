Draft ADR: Secret Management Strategy
ID: ADR-00X (Assign next available number)
Date: 2025-04-10
Status: Draft
Context
CowGnition, a desktop Go application acting as an MCP server for Remember The Milk (RTM), requires secure storage for sensitive credentials:

Application's RTM API Key and Shared Secret.  
User-specific RTM Auth Token obtained via OAuth-like flow. Storing these securely on the user's local machine is crucial. Current configuration reads keys from environment variables or potentially config files (cowgnition.yaml, claude_desktop_config.json), which is not ideal for persistent, secure storage on a desktop.  
Decision
Leverage OS-level secure storage mechanisms (Keychain, Credential Manager, Keyring) for storing all RTM credentials (API Key, Shared Secret, Auth Token). This will be implemented using a cross-platform Go library that abstracts the OS-specific APIs. The recommended library is zalando/go-keyring due to its cross-platform support and avoidance of C bindings.

Secrets will be stored using a unique service identifier (e.g., "CowGnitionRTM").
Individual secrets will be identified by key names (e.g., "RTM_API_KEY", "RTM_AUTH_TOKEN").
The application will retrieve secrets using keyring.Get() on startup or when needed.
Secrets will be stored using keyring.Set() after initial acquisition (e.g., via GUI input or successful auth flow).
Alternatives Considered
Environment Variables: Simpler but less secure on desktop; variables can be inspected or leaked. Current partial implementation.
Plaintext Config Files: Highly insecure, even with restricted file permissions (0600). Easy to accidentally expose.
Encrypted Config Files: Requires managing encryption keys securely, essentially shifting the problem.
1Password Integration (via CLI):
Pros: Centralized management in 1Password, leverages its security.
Cons: Requires user to have 1Password + CLI installed/configured, adds external dependency for the app to function.
GUI Prompt for Input:
Pros: User-friendly initial acquisition, password manager compatible.
Cons: Still requires a secure persistence mechanism (ideally the OS keychain) after initial input; otherwise, secrets are insecure or need re-entry.
Consequences
Positive:
Significantly improves security by using robust OS-level protection.
Removes secrets from easily accessible config files or source code.
Provides a consistent storage mechanism across macOS, Windows, and Linux (with appropriate keyring service).
Relatively simple integration using existing Go libraries.
Negative:
Adds a dependency on the chosen Go keyring library (zalando/go-keyring).
On Linux, requires a compatible D-Bus Secret Service (like gnome-keyring) to be installed and running.
Error handling for keychain/keyring access needs to be implemented (e.g., what happens if access is denied or the secret isn't found?).
