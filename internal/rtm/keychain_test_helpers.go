// Package rtm manages the connection to Remember The Milk
package rtm

// file: internal/rtm/keychain_test_helpers.go

import (
	"github.com/zalando/go-keyring"
)

// TestKeychainSet tests setting a value in the keychain.
func TestKeychainSet(service, user, value string) error {
	return keyring.Set(service, user, value)
}

// TestKeychainGet tests getting a value from the keychain.
func TestKeychainGet(service, user string) (string, error) {
	return keyring.Get(service, user)
}

// TestKeychainDelete tests deleting a value from the keychain.
func TestKeychainDelete(service, user string) error {
	return keyring.Delete(service, user)
}
