package vault

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/bitmaelum/bitmaelum-suite/internal/account"
	"github.com/bitmaelum/bitmaelum-suite/pkg/address"
	"github.com/juju/fslock"
	"github.com/mitchellh/go-homedir"
	"golang.org/x/crypto/pbkdf2"
	"io"
	"io/ioutil"
	"os"
	"path"
)

const (
	pbkdfIterations = 100002
)

// Vault defines our vault with path and password. Only the accounts should be exported
type Vault struct {
	Accounts []account.Info
	password []byte
	path     string
}

// New instantiates a new vault
func New(p string, pwd []byte) (*Vault, error) {
	p, err := homedir.Expand(p)
	if err != nil {
		return nil, err
	}

	v := &Vault{
		Accounts: []account.Info{},
		password: pwd,
		path:     p,
	}

	// Save new vault when we cannot find one
	if _, ok := err.(*os.PathError); ok {
		err = os.MkdirAll(path.Dir(p), 0777)
		if err != nil {
			return nil, err
		}
		err = v.Save()
		return v, nil
	}

	// Otherwise, read vault data
	err = v.unlockVault()
	if err != nil {
		return nil, err
	}

	return v, nil
}

type vaultJSONData struct {
	Data []byte `json:"data"`
	Salt []byte `json:"salt"`
	Iv   []byte `json:"iv"`
	Hmac []byte `json:"hmac"`
}

// unlockVault unlocks the vault by the given password
func (v *Vault) unlockVault() error {
	data, err := readFileData(v.path)
	if err != nil {
		return err
	}

	vaultData := &vaultJSONData{}
	err = json.Unmarshal(data, &vaultData)
	if err != nil {
		return err
	}

	// Check if HMAC is correct
	hash := hmac.New(sha256.New, v.password)
	hash.Write(vaultData.Data)
	if bytes.Compare(hash.Sum(nil), vaultData.Hmac) != 0 {
		return errors.New("incorrect password")
	}

	// Generate key based on password
	derivedAESKey := pbkdf2.Key(v.password, vaultData.Salt, pbkdfIterations, 32, sha256.New)
	aes256, err := aes.NewCipher(derivedAESKey)
	if err != nil {
		return err
	}

	// Decrypt vault data
	plainText := make([]byte, len(vaultData.Data))
	ctr := cipher.NewCTR(aes256, vaultData.Iv)
	ctr.XORKeyStream(plainText, vaultData.Data)

	// Unmarshal vault data
	var accounts []account.Info
	err = json.Unmarshal(plainText, &accounts)
	if err != nil {
		return err
	}

	v.Accounts = accounts
	return nil
}

// AddAccount adds a new account to the vault
func (v *Vault) AddAccount(account account.Info) {
	v.Accounts = append(v.Accounts, account)
}

// RemoveAccount removes the given account from the vault
func (v *Vault) RemoveAccount(addr address.Address) {
	k := 0
	for _, acc := range v.Accounts {
		if acc.Address != addr.String() {
			v.Accounts[k] = acc
			k++
		}
	}
	v.Accounts = v.Accounts[:k]
}

// Save saves the account data back into the vault on disk
func (v *Vault) Save() error {
	// Generate 64 byte salt
	salt := make([]byte, 64)
	_, err := io.ReadFull(rand.Reader, salt)
	if err != nil {
		return err
	}

	// Generate key based on password
	derivedAESKey := pbkdf2.Key(v.password, salt, pbkdfIterations, 32, sha256.New)
	aes256, err := aes.NewCipher(derivedAESKey)
	if err != nil {
		return err
	}

	// Generate 32 byte IV
	iv := make([]byte, aes.BlockSize)
	_, err = io.ReadFull(rand.Reader, iv)
	if err != nil {
		return err
	}

	// Marshal and encrypt the data
	plainText, err := json.MarshalIndent(&v.Accounts, "", "  ")
	if err != nil {
		return err
	}

	cipherText := make([]byte, len(plainText))
	ctr := cipher.NewCTR(aes256, iv)
	ctr.XORKeyStream(cipherText, plainText)

	// Generate HMAC based on the encrypted data (encrypt-then-mac?)
	hash := hmac.New(sha256.New, v.password)
	hash.Write(cipherText)

	// Generate the vault structure for disk
	data, err := json.MarshalIndent(&vaultJSONData{
		Data: cipherText,
		Salt: salt,
		Iv:   iv,
		Hmac: hash.Sum(nil),
	}, "", "  ")
	if err != nil {
		return err
	}

	// Write vault back through temp file
	return writeFileData(v.path, data, 0600)
}

// GetAccountInfo tries to find the given address and returns the account from the vault
func (v *Vault) GetAccountInfo(addr address.Address) (*account.Info, error) {
	for _, acc := range v.Accounts {
		if acc.Address == addr.String() {
			return &acc, nil
		}
	}

	return nil, errors.New("cannot find account")
}

// HasAccount returns true when the vault has an account for the given address
func (v *Vault) HasAccount(addr address.Address) bool {
	_, err := v.GetAccountInfo(addr)

	return err == nil
}

// writeFileData writes data by safely writing to a temp file first
func writeFileData(path string, data []byte, perm os.FileMode) error {
	// Lock the file first. Make sure we are the only one working on it
	lock := fslock.New(path + ".lock")
	err := lock.TryLock()
	if err != nil {
		return err
	}

	defer func() {
		_ = lock.Unlock()
		_ = os.Remove(path + ".lock")
	}()

	err = ioutil.WriteFile(path+".tmp", data, perm)
	if err != nil {
		return err
	}

	err = os.Rename(path+".tmp", path)
	return err
}

// Read file data
func readFileData(p string) ([]byte, error) {
	// Lock vault for reading
	lock := fslock.New(p + ".lock")
	err := lock.TryLock()
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(p)

	_ = lock.Unlock()
	_ = os.Remove(p + ".lock")
	if err != nil {
		return nil, err
	}

	return data, nil
}
