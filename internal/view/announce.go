package view

import (
	"strings"

	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

// Message texts, shared by the visible messages and their announcements.
const (
	validationTitle  = "Confira o nome da cidade"
	notFoundTitle    = "Cidade não encontrada"
	notFoundText     = "Não encontramos nenhuma cidade com esse nome. Confira a grafia ou tente uma cidade próxima e busque de novo."
	unavailableTitle = "Clima indisponível no momento"
	unavailableText  = "Não conseguimos obter os dados meteorológicos agora. Aguarde alguns instantes e busque de novo."
)

// The announcements are single sentences in reading order. Screen readers announce a live region that receives
// many elements at once piece by piece, in whatever order the browser reports them; one text change is read whole.

// ResultAnnouncement reads a result as one text: the location, then each reading with its label.
func ResultAnnouncement(r weather.Report) string {
	c := r.Conditions
	return strings.Join([]string{
		"Agora em " + formatLocation(r.Location) + ".",
		"Temperatura: " + formatTemperature(c.TemperatureC) + ".",
		"Condição: " + weather.ConditionDescription(c.WeatherCode) + ".",
		"Sensação térmica: " + formatTemperature(c.ApparentTemperatureC) + ".",
		"Umidade: " + formatHumidity(c.RelativeHumidityPct) + ".",
		"Vento: " + formatWindSpeed(c.WindSpeedKmh) + ".",
	}, " ")
}

// ValidationAnnouncement reads a validation message with its title.
func ValidationAnnouncement(message string) string { return validationTitle + ". " + message }

// NotFoundAnnouncement reads the city-not-found message with its title.
func NotFoundAnnouncement() string { return notFoundTitle + ". " + notFoundText }

// UnavailableAnnouncement reads the temporarily-unavailable message with its title.
func UnavailableAnnouncement() string { return unavailableTitle + ". " + unavailableText }
