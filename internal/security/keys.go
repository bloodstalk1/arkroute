package security

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

func GenerateClientKey() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "ark_" + base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
