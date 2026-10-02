package security

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

const (
	saltPostfix  = "@lomorage.lomoware"
	argonVersion = argon2.Version
	argonTime    = 3
	argonMemory  = 4096
	argonThread  = 1
	argonHashLen = 32
)

// EncryptPassword encrypt password.
func EncryptPassword(username, password string) string {
	salt := username + saltPostfix
	hash := argon2.IDKey([]byte(password), []byte(salt), argonTime, argonMemory, argonThread, argonHashLen)
	b64Salt := base64.RawStdEncoding.EncodeToString([]byte(salt))
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	// Return a string using the standard encoded hash representation.
	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argonVersion, argonMemory, argonTime, argonThread, b64Salt, b64Hash)
	return fmt.Sprintf("%x00", encodedHash)
}

// LomoPasswdToOSPasswd convert lomod password to os password.
func LomoPasswdToOSPasswd(password string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(password)))
}
