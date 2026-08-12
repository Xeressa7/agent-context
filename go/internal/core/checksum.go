package core

import (
	"math/big"
	"strings"
)

// A PII decision is made by CHECKSUM, not by shape.
//
// Measured during 0.5.0: shape-only patterns read ordinary prose as personal
// data — "2024 2025 2026 2027" (a run of years) looked like a card number, and
// "AB12CDEF1234GHIJ5678" (a key or hash) looked like an IBAN. A gate that cries
// wolf is not a gate: people learn to pass --no-verify, and then real PII gets
// through too. So shape only nominates a candidate; these functions decide.

// LuhnValid reports whether the digit string satisfies the Luhn checksum.
// Input must contain digits only; callers strip separators first.
func LuhnValid(digits string) bool {
	if digits == "" {
		return false
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		c := digits[i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if double {
			if d *= 2; d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// IBANValid reports whether the string is a structurally valid IBAN per the
// ISO 7064 mod-97 check. Country agnostic on purpose: the same code accepts TR,
// DE and GB accounts, so no country profile is needed for this class of PII.
//
// A big.Int is used rather than the running-remainder trick because the
// rearranged number can exceed 64 bits; correctness over cleverness here.
func IBANValid(raw string) bool {
	s := strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(raw))
	if len(s) < 15 || len(s) > 34 {
		return false
	}

	rotated := s[4:] + s[:4]
	var numeric strings.Builder
	numeric.Grow(len(rotated) * 2)
	for _, r := range rotated {
		switch {
		case r >= '0' && r <= '9':
			numeric.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			// A=10 … Z=35
			numeric.WriteString(string(rune('0' + (r-'A'+10)/10)))
			numeric.WriteString(string(rune('0' + (r-'A'+10)%10)))
		default:
			return false
		}
	}

	n, ok := new(big.Int).SetString(numeric.String(), 10)
	if !ok {
		return false
	}
	return n.Mod(n, big.NewInt(97)).Int64() == 1
}
