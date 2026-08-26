package mtn

import (
	"errors"
	"strings"
)

func normalizeMSISDN(account string) (string, error) {
	digits := strings.Map(func(r rune) rune {
		if strings.ContainsRune(" \t\r\n-()+.", r) {
			return -1
		}
		return r
	}, strings.TrimSpace(account))
	digits = strings.TrimPrefix(digits, "00")
	if strings.IndexFunc(digits, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return "", errors.New("account must be an MTN MoMo mobile number")
	}
	if len(digits) < 8 || len(digits) > 15 {
		return "", errors.New("account must contain 8 to 15 digits")
	}
	if strings.HasPrefix(digits, "0") {
		return "", errors.New("account must include its international country code")
	}
	return digits, nil
}
