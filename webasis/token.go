package webasis

import (
	"encoding/base64"
	"strings"
)

const token_sep = "\x1f"

func ToToken(name, secret string) string {
	return base64.URLEncoding.EncodeToString([]byte(name + token_sep + secret))
}

func FromToken(token string) (name, secret string) {
	raw, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", ""
	}

	data := strings.Split(string(raw), token_sep)
	if len(data) > 0 {
		name = data[0]
	}
	if len(data) > 1 {
		secret = data[1]
	}
	return
}
