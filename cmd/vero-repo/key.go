package main

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// A key's folder holds private.asc - the signing key, which only its owner
// can read - and key.asc, the public half, which the site publishes.
const (
	privateFile = "private.asc"
	publicFile  = "key.asc"
)

// keyBits is the size of a new RSA key: what apt, dnf, pkg and the rest
// all accept, on old systems as well as new.
var keyBits = 4096

// makeKey makes a signing key in dir, refusing to replace one.
func makeKey(dir, name, email string) error {
	if _, err := os.Stat(filepath.Join(dir, privateFile)); err == nil {
		return fmt.Errorf("%s already has a key; it signs every release, so it isn't replaced", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	cfg := &packet.Config{Algorithm: packet.PubKeyAlgoRSA, RSABits: keyBits, DefaultHash: crypto.SHA256}
	e, err := openpgp.NewEntity(name, "repository signing key", email, cfg)
	if err != nil {
		return err
	}
	var priv, pub bytes.Buffer
	w, err := armor.Encode(&priv, openpgp.PrivateKeyType, nil)
	if err != nil {
		return err
	}
	if err := e.SerializePrivate(w, cfg); err != nil {
		return err
	}
	w.Close()
	w, err = armor.Encode(&pub, openpgp.PublicKeyType, nil)
	if err != nil {
		return err
	}
	if err := e.Serialize(w); err != nil {
		return err
	}
	w.Close()
	if err := os.WriteFile(filepath.Join(dir, privateFile), priv.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, publicFile), pub.Bytes(), 0o644); err != nil {
		return err
	}
	// OpenBSD's packages are signed with signify, which has keys of its
	// own: one is made now, beside the OpenPGP key.
	_, err = (&signer{dir: dir}).signify()
	return err
}

// signer is a key that signs a release.
type signer struct {
	entity *openpgp.Entity
	public []byte
	// dir is the key's folder, which also keeps the signify key that
	// signs OpenBSD packages.
	dir string
}

func loadKey(dir string) (*signer, error) {
	f, err := os.Open(filepath.Join(dir, privateFile))
	if err != nil {
		return nil, fmt.Errorf("no signing key in %s: make one with vero-repo key", dir)
	}
	defer f.Close()
	list, err := openpgp.ReadArmoredKeyRing(f)
	if err != nil || len(list) != 1 || list[0].PrivateKey == nil {
		return nil, fmt.Errorf("%s isn't a signing key", filepath.Join(dir, privateFile))
	}
	if list[0].PrivateKey.Encrypted {
		return nil, errors.New("the signing key has a passphrase, which vero-repo doesn't ask for")
	}
	pub, err := os.ReadFile(filepath.Join(dir, publicFile))
	if err != nil {
		return nil, err
	}
	return &signer{entity: list[0], public: pub, dir: dir}, nil
}

var signConfig = &packet.Config{DefaultHash: crypto.SHA256}

// detach is an armored signature of data, as Release.gpg is.
func (s *signer) detach(data []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&out, s.entity, bytes.NewReader(data), signConfig); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// clearsign is data signed in place, as InRelease is.
func (s *signer) clearsign(data []byte) ([]byte, error) {
	var out bytes.Buffer
	w, err := clearsign.Encode(&out, s.entity.PrivateKey, signConfig)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
