package keychain

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

// Where there is no keychain, an app can keep its secrets' files
// encrypted with a password that the person chooses, as vero's apps do:
//
//	keychain.UsePassword(dir)   once, at start, with the app's folder
//
// From then on, where there is no keychain, an Item kept in a file is
// encrypted with a key made from the password, and Load and Save return
// ErrLocked until the password is given with Unlock. Until a password is
// set, with SetPassword, Save returns ErrNeedsPassword, so that nothing new
// is written unencrypted; a file from before is still read, and encrypted
// when it is next saved. Where there is a keychain, none of this applies.
//
// A worker answers the front end's questions about it with the standard
// requests, which vero's libraries ask:
//
//	vero.Handle(w, keychain.StatusRequest, keychain.StatusHandler)
//	vero.Handle(w, keychain.UnlockRequest, keychain.UnlockHandler)
//	vero.Handle(w, keychain.PasswordRequest, keychain.PasswordHandler)

// ErrLocked is a secret kept in a file that needs the password, which has
// not been given.
var ErrLocked = errors.New("the sign-ins are locked: their password is needed")

// ErrNeedsPassword is a secret that cannot be saved until a password is
// chosen, since there is no keychain to keep it in.
var ErrNeedsPassword = errors.New("there is no keychain here, so choose a password for the sign-ins first")

// ErrWrongPassword is a password that is not the one set.
var ErrWrongPassword = errors.New("that is not the password")

// The standard requests' names.
const (
	StatusRequest   = "vero.keychain"
	UnlockRequest   = "vero.unlock"
	PasswordRequest = "vero.setPassword"
)

// Status is how the secrets are kept, as the front end needs to know it.
type Status struct {
	// Keychain says the system's keychain keeps them, and no password is
	// needed.
	Keychain bool `json:"keychain"`
	// NeedsPassword says there is no keychain and no password yet: the
	// front end asks the person to choose one.
	NeedsPassword bool `json:"needsPassword"`
	// Locked says a password is set and has not been given: the front end
	// asks for it.
	Locked bool `json:"locked"`
}

var magic = []byte("vero-keychain-1\n")

// passwords is the state of the password, for the whole program.
var passwords struct {
	sync.Mutex
	dir string
	key []byte
	// noKeychain behaves as if there were no keychain, for tests on a
	// system that has one.
	noKeychain bool
}

// check is the file beside the secrets that says a password is set: the
// salt its key is made with, a short text encrypted with that key, which
// tells a right password from a wrong one, and the files encrypted with it.
type check struct {
	Salt  []byte   `json:"salt"`
	Check []byte   `json:"check"`
	Files []string `json:"files"`
}

const checkName = "keychain-password.json"

// UsePassword keeps secrets' files encrypted with a password, where there
// is no keychain. dir is the app's folder, such as kit/profile's Dir, in
// which a small file records the password's salt.
func UsePassword(dir string) {
	passwords.Lock()
	defer passwords.Unlock()
	passwords.dir = dir
}

// protecting says whether files are kept encrypted: UsePassword was called
// and there is no keychain. The caller holds passwords' lock.
func protecting() bool { return passwords.dir != "" && (!available || passwords.noKeychain) }

// protected is protecting, taking the lock.
func protected() bool {
	passwords.Lock()
	defer passwords.Unlock()
	return protecting()
}

func readCheck() (*check, error) {
	data, err := os.ReadFile(filepath.Join(passwords.dir, checkName))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c check
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func writeCheck(c *check) error {
	data, _ := json.MarshalIndent(c, "", "  ")
	return writeFile(filepath.Join(passwords.dir, checkName), data)
}

// keyFor is the key a password makes with a salt: Argon2id, with enough
// memory and time that guessing is slow, and little enough for a
// Raspberry Pi.
func keyFor(password string, salt []byte) []byte {
	return argon2.IDKey([]byte(password), salt, 3, 64*1024, 4, chacha20poly1305.KeySize)
}

func seal(key, plain []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(append(slices.Clone(magic), nonce...), aead.Seal(nil, nonce, plain, nil)...), nil
}

func openSealed(key, sealed []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	sealed = bytes.TrimPrefix(sealed, magic)
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("keychain: a damaged file")
	}
	return aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
}

// CurrentStatus is how the secrets are kept now.
func CurrentStatus() Status {
	passwords.Lock()
	defer passwords.Unlock()
	if !protecting() {
		return Status{Keychain: available && !passwords.noKeychain}
	}
	c, _ := readCheck()
	return Status{NeedsPassword: c == nil, Locked: c != nil && passwords.key == nil}
}

// Unlock gives the password, which then stays known until Lock.
func Unlock(password string) error {
	passwords.Lock()
	defer passwords.Unlock()
	if !protecting() {
		return nil
	}
	c, err := readCheck()
	if err != nil {
		return err
	}
	if c == nil {
		return ErrNeedsPassword
	}
	key := keyFor(password, c.Salt)
	if text, err := openSealed(key, c.Check); err != nil || string(text) != "vero" {
		return ErrWrongPassword
	}
	passwords.key = key
	return nil
}

// Lock forgets the password, until Unlock gives it again.
func Lock() {
	passwords.Lock()
	defer passwords.Unlock()
	passwords.key = nil
}

// SetPassword chooses the password, or changes it: old is the password
// set before, or "" the first time. Every file encrypted with the old one
// is encrypted again with the new.
func SetPassword(old, password string) error {
	if len(password) < 8 {
		return errors.New("choose a password of at least eight characters")
	}
	passwords.Lock()
	defer passwords.Unlock()
	if !protecting() {
		return nil
	}
	c, err := readCheck()
	if err != nil {
		return err
	}
	var oldKey []byte
	if c != nil {
		oldKey = keyFor(old, c.Salt)
		if text, err := openSealed(oldKey, c.Check); err != nil || string(text) != "vero" {
			return ErrWrongPassword
		}
	} else {
		c = &check{}
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	key := keyFor(password, salt)
	// Every file encrypted with the old key, encrypted again with the new.
	for _, path := range c.Files {
		sealed, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		plain, err := openSealed(oldKey, sealed)
		if err != nil {
			return err
		}
		again, err := seal(key, plain)
		if err != nil {
			return err
		}
		if err := writeFile(path, again); err != nil {
			return err
		}
	}
	sealedCheck, err := seal(key, []byte("vero"))
	if err != nil {
		return err
	}
	c.Salt, c.Check = salt, sealedCheck
	if err := writeCheck(c); err != nil {
		return err
	}
	passwords.key = key
	return nil
}

// loadProtected reads an encrypted file, or a plain one from before a
// password was used, which is encrypted at once when it can be.
func loadProtected(path string) ([]byte, error) {
	passwords.Lock()
	defer passwords.Unlock()
	data, err := readFile(path)
	if err != nil || data == nil {
		return data, err
	}
	if !bytes.HasPrefix(data, magic) {
		if passwords.key != nil {
			saveProtectedLocked(path, data)
		}
		return data, nil
	}
	if passwords.key == nil {
		return nil, ErrLocked
	}
	return openSealed(passwords.key, data)
}

func saveProtected(path string, data []byte) error {
	passwords.Lock()
	defer passwords.Unlock()
	return saveProtectedLocked(path, data)
}

func saveProtectedLocked(path string, data []byte) error {
	if passwords.key == nil {
		if c, _ := readCheck(); c == nil {
			return ErrNeedsPassword
		}
		return ErrLocked
	}
	sealed, err := seal(passwords.key, data)
	if err != nil {
		return err
	}
	if err := writeFile(path, sealed); err != nil {
		return err
	}
	c, err := readCheck()
	if err != nil || c == nil {
		return err
	}
	if abs, _ := filepath.Abs(path); !slices.Contains(c.Files, abs) {
		c.Files = append(c.Files, abs)
		return writeCheck(c)
	}
	return nil
}

// StatusHandler answers StatusRequest: how the secrets are kept.
func StatusHandler(context.Context, struct{}) (Status, error) { return CurrentStatus(), nil }

// UnlockHandler answers UnlockRequest, with the password.
func UnlockHandler(_ context.Context, req struct {
	Password string `json:"password"`
}) (Status, error) {
	if err := Unlock(req.Password); err != nil {
		return Status{}, err
	}
	return CurrentStatus(), nil
}

// PasswordHandler answers PasswordRequest: the password chosen, or
// changed from old.
func PasswordHandler(_ context.Context, req struct {
	Old      string `json:"old"`
	Password string `json:"password"`
}) (Status, error) {
	if err := SetPassword(req.Old, req.Password); err != nil {
		return Status{}, err
	}
	return CurrentStatus(), nil
}
