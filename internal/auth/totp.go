package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func NewTOTPSecret() string {
	var b [20]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]), "=")
}

func VerifyTOTP(secret, code string, now time.Time) bool {
	secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	code = strings.TrimSpace(code)
	if secret == "" || len(code) != 6 {
		return false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return false
	}
	counter := now.UTC().Unix() / 30
	for drift := int64(-1); drift <= 1; drift++ {
		if totpCode(key, uint64(counter+drift)) == code {
			return true
		}
	}
	return false
}

func CurrentTOTPCode(secret string, now time.Time) (string, bool) {
	secret = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	if secret == "" {
		return "", false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return "", false
	}
	return totpCode(key, uint64(now.UTC().Unix()/30)), true
}

func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	query := url.Values{
		"digits": []string{"6"},
		"issuer": []string{issuer},
		"period": []string{"30"},
		"secret": []string{secret},
	}
	return fmt.Sprintf("otpauth://totp/%s?%s", label, query.Encode())
}

func NewRecoveryCodes(count int) []string {
	out := make([]string, 0, count)
	for len(out) < count {
		var b [5]byte
		if _, err := rand.Read(b[:]); err != nil {
			out = append(out, fmt.Sprintf("%010d", time.Now().UnixNano()%1_000_000_0000))
			continue
		}
		out = append(out, fmt.Sprintf("%x-%x", b[:2], b[2:]))
	}
	return out
}

func totpCode(key []byte, counter uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(buf[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		(uint32(sum[offset+1])&0xff)<<16 |
		(uint32(sum[offset+2])&0xff)<<8 |
		(uint32(sum[offset+3]) & 0xff)
	code := value % 1_000_000
	return fmt.Sprintf("%06d", code)
}
