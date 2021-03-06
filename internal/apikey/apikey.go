package apikey

import (
	"github.com/bitmaelum/bitmaelum-suite/internal/encrypt"
	"strings"
	"time"
)

// KeyType represents a key with a validity and permissions. When admin is true, permission checks are always true
type KeyType struct {
	ID          string    `json:"key"`
	ValidUntil  time.Time `json:"valid_until"`
	Permissions []string  `json:"permissions"`
	Admin       bool      `json:"admin"`
}

// NewAdminKey creates a new admin key
func NewAdminKey(valid time.Duration) KeyType {
	var until = time.Time{}
	if valid > 0 {
		until = time.Now().Add(valid)
	}

	return KeyType{
		ID:          encrypt.GenerateKey("BMK-", 32),
		ValidUntil:  until,
		Permissions: nil,
		Admin:       true,
	}
}

// NewKey creates a new key with the given permissions and duration
func NewKey(perms []string, valid time.Duration) KeyType {
	var until = time.Time{}
	if valid > 0 {
		until = time.Now().Add(valid)
	}

	return KeyType{
		ID:          encrypt.GenerateKey("BMK-", 32),
		ValidUntil:  until,
		Permissions: perms,
		Admin:       false,
	}
}

// HasPermission returns true when this key contains the given permission, or when the key is an admin key (granted all permissions)
func (key *KeyType) HasPermission(perm string) bool {
	if key.Admin == true {
		return true
	}

	perm = strings.ToLower(perm)

	for _, p := range key.Permissions {
		if strings.ToLower(p) == perm {
			return true
		}
	}

	return false
}

// Repository is a repository to fetch and store API keys
type Repository interface {
	Fetch(ID string) (*KeyType, error)
	Store(key KeyType) error
	Remove(ID string)
}
