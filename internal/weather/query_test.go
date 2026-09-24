package weather_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

func TestParseQueryAccepts(t *testing.T) {
	t.Parallel()

	const decomposedSaoPaulo = "São Paulo" // "a" followed by a combining tilde
	tests := []struct {
		name, input string
		want        weather.Query
	}{
		{"plain name", "Recife", weather.Query{Text: "Recife", Key: "recife"}},
		{"two letters with an accent", "Ré", weather.Query{Text: "Ré", Key: "ré"}},
		{"two digits", "10", weather.Query{Text: "10", Key: "10"}},
		{"surrounding whitespace", "  Recife \t", weather.Query{Text: "Recife", Key: "recife"}},
		{"internal whitespace runs", "São   Paulo", weather.Query{Text: "São Paulo", Key: "são paulo"}},
		{"tabs, newlines, and no-break spaces", "São\t\n Paulo", weather.Query{Text: "São Paulo", Key: "são paulo"}},
		{"decomposed accents", decomposedSaoPaulo, weather.Query{Text: "São Paulo", Key: "são paulo"}},
		{"upper case", "SÃO PAULO", weather.Query{Text: "SÃO PAULO", Key: "são paulo"}},
		{"non-Latin script", "東京", weather.Query{Text: "東京", Key: "東京"}},
		{"punctuation inside a name", "Sant'Ana do Livramento", weather.Query{
			Text: "Sant'Ana do Livramento", Key: "sant'ana do livramento",
		}},
		{"control characters", "São\x00Paulo\x1b", weather.Query{Text: "São Paulo", Key: "são paulo"}},
		{"invalid UTF-8", "São\xffPaulo", weather.Query{Text: "São Paulo", Key: "são paulo"}},
		{"exactly 100 runes", strings.Repeat("á", 100), weather.Query{
			Text: strings.Repeat("á", 100), Key: strings.Repeat("á", 100),
		}},
		{"100 runes once composed", strings.Repeat("á", 100), weather.Query{
			Text: strings.Repeat("á", 100), Key: strings.Repeat("á", 100),
		}},
		{"100 runes once whitespace collapses", "  " + strings.Repeat("a", 100) + "  ", weather.Query{
			Text: strings.Repeat("a", 100), Key: strings.Repeat("a", 100),
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := weather.ParseQuery(tt.input)
			if err != nil {
				t.Fatalf("ParseQuery(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseQueryComposedAndDecomposedMatch(t *testing.T) {
	t.Parallel()

	composed, err := weather.ParseQuery("Goiânia")
	if err != nil {
		t.Fatalf("ParseQuery(composed) error = %v", err)
	}
	decomposed, err := weather.ParseQuery("Goiânia")
	if err != nil {
		t.Fatalf("ParseQuery(decomposed) error = %v", err)
	}
	if composed != decomposed {
		t.Errorf("composed = %+v, decomposed = %+v, want them equal", composed, decomposed)
	}
}

func TestParseQueryRejects(t *testing.T) {
	t.Parallel()

	const (
		empty = "Digite o nome de uma cidade para buscar o clima."
		short = "O nome da cidade precisa ter pelo menos duas letras ou números. Complete o nome e busque de novo."
		long  = "O nome da cidade pode ter no máximo 100 caracteres. Encurte o nome e busque de novo."
	)
	tests := []struct {
		name, input, wantMessage string
	}{
		{"empty", "", empty},
		{"whitespace only", " \t\n  ", empty},
		{"control characters only", "\x00\x1b", empty},
		{"single letter", "a", short},
		{"single accented letter", "é", short},
		{"single letter with whitespace", "  a  ", short},
		{"single letter with punctuation", "a.", short},
		{"punctuation only", "-.,;!?", short},
		{"separated punctuation", "- . ,", short},
		{"101 runes", strings.Repeat("a", 101), long},
		{"101 runes once composed", strings.Repeat("á", 101), long},
		{"101 runes with spaces", strings.Repeat("ab ", 33) + "ab", long},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := weather.ParseQuery(tt.input)
			if err == nil {
				t.Fatalf("ParseQuery(%q) = %+v, want a ValidationError", tt.input, got)
			}
			var verr weather.ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("ParseQuery(%q) error = %T %v, want a ValidationError", tt.input, err, err)
			}
			if verr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", verr.Message, tt.wantMessage)
			}
			if err.Error() != tt.wantMessage {
				t.Errorf("Error() = %q, want %q", err.Error(), tt.wantMessage)
			}
			if got != (weather.Query{}) {
				t.Errorf("ParseQuery(%q) = %+v, want the zero Query on error", tt.input, got)
			}
		})
	}
}
