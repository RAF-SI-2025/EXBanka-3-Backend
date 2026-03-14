package util

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
var digitsOnlyRegex = regexp.MustCompile(`^\d+$`)

func ValidateEmail(email string) error {
	if strings.TrimSpace(email) == "" {
		return errors.New("email is required")
	}
	if !emailRegex.MatchString(email) {
		return errors.New("invalid email format")
	}
	return nil
}

func ValidatePhoneNumber(phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil
	}
	if !digitsOnlyRegex.MatchString(phone) {
		return errors.New("phone number must contain only digits")
	}
	return nil
}

func ValidateBankEmail(email string) error {
	if err := ValidateEmail(email); err != nil {
		return err
	}
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(email)), "@bank.com") {
		return errors.New("email must end with @bank.com")
	}
	return nil
}

func ValidateDateOfBirth(dob time.Time) error {
	if dob.After(time.Now()) {
		return errors.New("date_of_birth must not be in the future")
	}
	return nil
}
