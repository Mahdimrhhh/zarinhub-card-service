package validation

import (
	"errors"
	"strings"
)

const CardLength = 16

var (
	ErrEmptyCard       = errors.New("card number is empty")
	ErrNonNumeric      = errors.New("card number must contain only digits")
	ErrInvalidLength   = errors.New("card number must contain 16 digits")
	ErrInvalidChecksum = errors.New("card number failed Luhn checksum")
)

func Validate(card string) error {
	if card == "" {
		return ErrEmptyCard
	}
	for _, char := range card {
		if char < '0' || char > '9' {
			return ErrNonNumeric
		}
	}
	if len(card) != CardLength {
		return ErrInvalidLength
	}
	if !passesLuhn(card) {
		return ErrInvalidChecksum
	}
	return nil
}

func passesLuhn(card string) bool {
	sum := 0
	double := false

	for index := len(card) - 1; index >= 0; index-- {
		digit := int(card[index] - '0')
		if double {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
		double = !double
	}

	return sum%10 == 0
}

func Mask(card string) string {
	if len(card) < 8 || !containsOnlyDigits(card) {
		return "********"
	}
	return card[:4] + strings.Repeat("*", len(card)-8) + card[len(card)-4:]
}

func containsOnlyDigits(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
