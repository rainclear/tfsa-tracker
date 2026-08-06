package utils

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidCurrency = errors.New("invalid currency format")

// DollarsToCents parses string inputs into integer cents without floating-point math.
// Examples: "123.45" -> 12345, "50" -> 5000, "0.5" -> 50, "100.00" -> 10000
func DollarsToCents(input string) (int64, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return 0, ErrInvalidCurrency
	}

	parts := strings.Split(input, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidCurrency
	}

	dollarsStr := parts[0]
	if dollarsStr == "" {
		dollarsStr = "0"
	}

	dollars, err := strconv.ParseInt(dollarsStr, 10, 64)
	if err != nil || dollars < 0 {
		return 0, ErrInvalidCurrency
	}

	var cents int64 = 0
	if len(parts) == 2 {
		centsStr := parts[1]
		if len(centsStr) == 1 {
			centsStr += "0"
		} else if len(centsStr) > 2 {
			centsStr = centsStr[:2] // Truncate sub-cents
		}

		parsedCents, err := strconv.ParseInt(centsStr, 10, 64)
		if err != nil || parsedCents < 0 {
			return 0, ErrInvalidCurrency
		}
		cents = parsedCents
	}

	totalCents := dollars*100 + cents
	if totalCents <= 0 {
		return 0, fmt.Errorf("amount must be greater than zero")
	}

	return totalCents, nil
}

func FormatCurrency(cents int64) string {
	dollars := float64(cents) / 100.0
	str := fmt.Sprintf("%.2f", dollars)
	parts := strings.Split(str, ".")

	intPart := parts[0]
	decPart := parts[1]

	var result strings.Builder
	length := len(intPart)
	for i, c := range intPart {
		if i > 0 && (length-i)%3 == 0 {
			result.WriteRune(',')
		}
		result.WriteRune(c)
	}

	return fmt.Sprintf("$%s.%s", result.String(), decPart)
}
