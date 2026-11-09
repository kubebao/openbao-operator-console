package localauth

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Password rules follow NIST SP 800-63B rev. 4 for single-factor passwords: a length
// minimum, a generous maximum, no composition rules, and a check against common,
// repetitive, and context-specific values. TOTP adds the second factor.
const (
	MinPasswordLength = 15
	// MaxPasswordBytes bounds the hashing input. It allows at least 64 characters in
	// any script.
	MaxPasswordBytes = 1024
	// minDistinctiveLength is the length a password must keep after removing the
	// username and console names.
	minDistinctiveLength = 8
)

// PasswordProblem is a bounded reason for rejecting a new password.
type PasswordProblem string

const (
	PasswordTooShort   PasswordProblem = "tooShort"
	PasswordTooLong    PasswordProblem = "tooLong"
	PasswordCommon     PasswordProblem = "common"
	PasswordRepetitive PasswordProblem = "repetitive"
	PasswordContextual PasswordProblem = "contextual"
)

// PasswordError reports why a new password was rejected.
type PasswordError struct{ Problem PasswordProblem }

func (e PasswordError) Error() string {
	switch e.Problem {
	case PasswordTooShort:
		return "use at least 15 characters"
	case PasswordTooLong:
		return "use at most 1024 bytes"
	case PasswordCommon:
		return "choose a less common password"
	case PasswordRepetitive:
		return "avoid repeated characters, sequences, and keyboard patterns"
	case PasswordContextual:
		return "do not base the password on the username or the console name"
	default:
		return "choose a different password"
	}
}

// contextWords are names an attacker tries first for this console.
var contextWords = []string{"openbao", "kubebao", "console", "operator", "password", "admin"}

// commonPasswords lists well-known passwords that meet the length minimum. Shorter
// common passwords already fail the length rule.
var commonPasswords = map[string]bool{
	"passwordpassword":                    true,
	"passwordpassword1":                   true,
	"password1234567890":                  true,
	"iloveyouiloveyou":                    true,
	"letmeinletmeinletmein":               true,
	"correcthorsebatterystaple":           true,
	"thequickbrownfoxjumpsoverthelazydog": true,
	"qwertyuiopasdfghjklzxcvbnm":          true,
	"abcdefghijklmnopqrstuvwxyz":          true,
	"administrator123":                    true,
	"changemechangeme":                    true,
	"welcomewelcome1":                     true,
	"trustno1trustno1":                    true,
}

var keyboardRows = []string{"`1234567890-=", "qwertyuiop[]\\", "asdfghjkl;'", "zxcvbnm,./"}

// CheckPassword validates a new password for username. It normalizes the password the
// same way hashing does.
func CheckPassword(password, username string) error {
	normalized := Normalize(password)
	if len(normalized) > MaxPasswordBytes {
		return PasswordError{PasswordTooLong}
	}
	if utf8.RuneCountInString(normalized) < MinPasswordLength {
		return PasswordError{PasswordTooShort}
	}
	folded := strings.ToLower(normalized)
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsPunct(r) && r != '\'' {
			return -1
		}
		return r
	}, folded)
	if commonPasswords[compact] {
		return PasswordError{PasswordCommon}
	}
	if repetitive(compact) || patterned(folded) {
		return PasswordError{PasswordRepetitive}
	}
	remaining := folded
	words := append([]string{strings.ToLower(username)}, contextWords...)
	for _, word := range words {
		if utf8.RuneCountInString(word) >= 3 {
			remaining = strings.ReplaceAll(remaining, word, "")
		}
	}
	distinctive := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, remaining)
	if utf8.RuneCountInString(distinctive) < minDistinctiveLength {
		return PasswordError{PasswordContextual}
	}
	return nil
}

// repetitive reports a password with few distinct characters or one built from a
// repeated unit of up to four characters.
func repetitive(value string) bool {
	runes := []rune(value)
	distinct := map[rune]bool{}
	for _, r := range runes {
		distinct[r] = true
	}
	if len(distinct) < 5 {
		return true
	}
	for unit := 1; unit <= 4 && unit < len(runes); unit++ {
		repeated := true
		for i := unit; i < len(runes); i++ {
			if runes[i] != runes[i%unit] {
				repeated = false
				break
			}
		}
		if repeated {
			return true
		}
	}
	return false
}

// patterned reports a password in which most adjacent characters are alphabet or
// digit sequences, or neighbours on a keyboard row.
func patterned(value string) bool {
	runes := []rune(value)
	if len(runes) < 2 {
		return false
	}
	steps := 0
	for i := 1; i < len(runes); i++ {
		delta := runes[i] - runes[i-1]
		if delta == 1 || delta == -1 || keyboardNeighbours(runes[i-1], runes[i]) {
			steps++
		}
	}
	return steps*5 >= (len(runes)-1)*4
}

func keyboardNeighbours(a, b rune) bool {
	for _, row := range keyboardRows {
		i, j := strings.IndexRune(row, a), strings.IndexRune(row, b)
		if i >= 0 && j >= 0 && (i-j == 1 || j-i == 1) {
			return true
		}
	}
	return false
}
