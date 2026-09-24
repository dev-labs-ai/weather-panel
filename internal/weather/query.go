package weather

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// maxQueryRunes caps the city input. It protects the service and the upstream API; no real place name comes close.
const maxQueryRunes = 100

// minMeaningfulRunes is the fewest letters or digits a search needs. The Geocoding API returns nothing for
// one-character names.
const minMeaningfulRunes = 2

// Query is a validated city search.
type Query struct {
	// Text is the normalized input sent to the Geocoding API.
	Text string
	// Key is Text in lower case, which identifies the search in the cache.
	Key string
}

// ValidationError reports input that cannot be searched. Message is shown to the user as is and says how to fix the
// search.
type ValidationError struct{ Message string }

func (e ValidationError) Error() string { return e.Message }

// User-facing validation messages, in Brazilian Portuguese like the rest of the UI.
const (
	msgEmptyQuery = "Digite o nome de uma cidade para buscar o clima."
	msgShortQuery = "O nome da cidade precisa ter pelo menos duas letras ou números. Complete o nome e busque de novo."
	msgLongQuery  = "O nome da cidade pode ter no máximo 100 caracteres. Encurte o nome e busque de novo."
)

// ParseQuery normalizes the city input and validates it. It trims the input, collapses internal whitespace into
// single spaces, and normalizes it to NFC, so composed and decomposed accents give the same Query. Control characters
// and invalid UTF-8 count as whitespace. It returns a ValidationError when the result has fewer than two letters or
// digits or more than 100 runes.
func ParseQuery(input string) (Query, error) {
	text := strings.Join(strings.FieldsFunc(input, isSeparator), " ")
	text = norm.NFC.String(text)

	if text == "" {
		return Query{}, ValidationError{Message: msgEmptyQuery}
	}
	meaningful := 0
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			meaningful++
		}
	}
	if meaningful < minMeaningfulRunes {
		return Query{}, ValidationError{Message: msgShortQuery}
	}
	if utf8.RuneCountInString(text) > maxQueryRunes {
		return Query{}, ValidationError{Message: msgLongQuery}
	}
	return Query{Text: text, Key: strings.ToLower(text)}, nil
}

// isSeparator reports whether r splits words in the input. Ranging over invalid UTF-8 yields utf8.RuneError for each
// bad byte, so those bytes are dropped as well.
func isSeparator(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsControl(r) || r == utf8.RuneError
}
