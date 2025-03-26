// internal/config/config.go
package config

import (
	"fmt"
)

// Config represents the application configuration.
type Config struct {
	Server ServerConfig
}

// ServerConfig contains server configuration.
type ServerConfig struct {
	Name string
	Port int
}

// NewConfig creates a new configuration with default values.
func NewConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Name: "CowGnition MCP Server",
			Port: 8080,
		},
	}
}

// GetServerName returns the server name.
func (c *Config) GetServerName() string {
	return c.Server.Name
}

// GetServerAddress returns the server address as host:port.
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf(":%d", c.Server.Port)
}
