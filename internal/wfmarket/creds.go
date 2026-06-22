package wfmarket

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
)

// Credentials are the saved warframe.market login. The password is obfuscated
// (NOT securely encrypted) so it isn't sitting in plain text; for real secrecy
// an OS keyring would be needed. Stored under the config dir, file mode 0600.
type Credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

const credsFile = "wfmarket-credentials.json"

// xorKey obfuscates the stored password. This is deliberately weak — it only
// guards against casual plain-text exposure.
var xorKey = []byte("wfo-companion-obfuscation-key")

func obfuscate(s string) string {
	b := []byte(s)
	for i := range b {
		b[i] ^= xorKey[i%len(xorKey)]
	}
	return base64.StdEncoding.EncodeToString(b)
}

func deobfuscate(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return ""
	}
	for i := range b {
		b[i] ^= xorKey[i%len(xorKey)]
	}
	return string(b)
}

// SaveCredentials writes the login to configDir (password obfuscated).
func SaveCredentials(configDir string, c Credentials) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	stored := Credentials{Email: c.Email, Password: obfuscate(c.Password)}
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, credsFile), data, 0o600)
}

// LoadCredentials reads the saved login, if any.
func LoadCredentials(configDir string) (Credentials, bool) {
	data, err := os.ReadFile(filepath.Join(configDir, credsFile))
	if err != nil {
		return Credentials{}, false
	}
	var stored Credentials
	if json.Unmarshal(data, &stored) != nil || stored.Email == "" {
		return Credentials{}, false
	}
	return Credentials{Email: stored.Email, Password: deobfuscate(stored.Password)}, true
}

// ClearCredentials removes the saved login.
func ClearCredentials(configDir string) error {
	return os.Remove(filepath.Join(configDir, credsFile))
}
