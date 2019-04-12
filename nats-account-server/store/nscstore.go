package store

import (
	"fmt"

	nsc "github.com/nats-io/nsc/cmd/store"
)

// NSCJWTStore implements the JWT Store interface, keeping all data in NSCory
type NSCJWTStore struct {
	nsc *nsc.Store
}

// NewNSCJWTStore returns an empty, immutable NSC folder-based JWT store
func NewNSCJWTStore(dirPath string) (JWTStore, error) {

	nscStore, err := nsc.LoadStore(dirPath)

	if err != nil {
		return nil, err
	}

	return &NSCJWTStore{
		nsc: nscStore,
	}, nil
}

// Load checks the NSCory store and returns the matching JWT or an error
func (store *NSCJWTStore) Load(publicKey string) (string, error) {

	infos, err := store.nsc.List(nsc.Accounts)
	if err != nil {
		return "", err
	}

	for _, i := range infos {
		if i.IsDir() {
			c, err := store.nsc.LoadClaim(nsc.Accounts, i.Name(), nsc.JwtName(i.Name()))
			if err != nil {
				return "", err
			}

			if c != nil {
				if c.Subject == publicKey {

					data, err := store.nsc.Read(nsc.Accounts, i.Name(), nsc.JwtName(i.Name()))
					if err != nil {
						return "", err
					}

					return string(data), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no matching JWT found")
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *NSCJWTStore) Save(publicKey string, theJWT string) error {
	return fmt.Errorf("store is read-only")
}

// IsReadOnly returns a flag determined at creation time
func (store *NSCJWTStore) IsReadOnly() bool {
	return true
}