// Package toolkit provides the encode/decode/hash helpers behind myTraffic's
// Decoder panel and the `mytraffic decode` CLI command.
package toolkit

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Transform applies a named transform to input and returns the result.
//
// Supported ops: base64encode, base64decode, urlencode, urldecode,
// hexencode, hexdecode, md5, sha1, sha256, jwtdecode.
func Transform(op, input string) (string, error) {
	switch strings.ToLower(op) {
	case "base64encode", "b64e":
		return base64.StdEncoding.EncodeToString([]byte(input)), nil
	case "base64decode", "b64d":
		b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(input))
		if err != nil {
			// Try URL-safe / unpadded as a fallback.
			if b2, e2 := base64.RawURLEncoding.DecodeString(strings.TrimSpace(input)); e2 == nil {
				return string(b2), nil
			}
			return "", err
		}
		return string(b), nil
	case "urlencode", "urle":
		return url.QueryEscape(input), nil
	case "urldecode", "urld":
		s, err := url.QueryUnescape(input)
		return s, err
	case "hexencode", "hexe":
		return hex.EncodeToString([]byte(input)), nil
	case "hexdecode", "hexd":
		b, err := hex.DecodeString(strings.TrimSpace(input))
		return string(b), err
	case "md5":
		h := md5.Sum([]byte(input))
		return hex.EncodeToString(h[:]), nil
	case "sha1":
		h := sha1.Sum([]byte(input))
		return hex.EncodeToString(h[:]), nil
	case "sha256":
		h := sha256.Sum256([]byte(input))
		return hex.EncodeToString(h[:]), nil
	case "jwtdecode", "jwt":
		return decodeJWT(input)
	default:
		return "", fmt.Errorf("unknown op %q", op)
	}
}

// Ops lists the available transforms.
func Ops() []string {
	return []string{
		"base64encode", "base64decode", "urlencode", "urldecode",
		"hexencode", "hexdecode", "md5", "sha1", "sha256", "jwtdecode",
	}
}

func decodeJWT(tok string) (string, error) {
	parts := strings.Split(strings.TrimSpace(tok), ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("not a JWT")
	}
	dec := func(s string) string {
		b, err := base64.RawURLEncoding.DecodeString(s)
		if err != nil {
			return s
		}
		var pretty any
		if json.Unmarshal(b, &pretty) == nil {
			out, _ := json.MarshalIndent(pretty, "", "  ")
			return string(out)
		}
		return string(b)
	}
	return "header:\n" + dec(parts[0]) + "\n\npayload:\n" + dec(parts[1]), nil
}
