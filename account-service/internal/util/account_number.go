package util

import (
	"fmt"
	"math/rand"
	"strconv"
)

const BankCode = "000"

// GenerateAccountNumber generates a random valid 18-digit account number.
// Format: BBB (3-digit bank code) + CCCCCCCCCCCCCC (13-digit account) + KK (2-digit check digits).
// Check digits satisfy: full_number mod 97 == 1 (ISO 7064 MOD 97-10).
func GenerateAccountNumber() string {
	accountPart := fmt.Sprintf("%013d", rand.Int63n(9_000_000_000_000)+1_000_000_000_000)
	base := BankCode + accountPart
	return base + checkDigits(base)
}

// ValidateAccountNumber returns true if the number is a valid 18-digit account number.
func ValidateAccountNumber(number string) bool {
	if len(number) != 18 {
		return false
	}
	for _, c := range number {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.ParseUint(number, 10, 64)
	if err != nil {
		return false
	}
	return n%97 == 1
}

func checkDigits(base string) string {
	n, _ := strconv.ParseUint(base+"00", 10, 64)
	kk := 98 - n%97
	return fmt.Sprintf("%02d", kk)
}
