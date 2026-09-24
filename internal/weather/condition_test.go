package weather_test

import (
	"strconv"
	"testing"

	"github.com/dev-labs-ai/weather-panel/internal/weather"
)

func TestConditionDescription(t *testing.T) {
	t.Parallel()

	for code, want := range map[int]string{
		0:  "Céu limpo",
		1:  "Predominantemente limpo",
		2:  "Parcialmente nublado",
		3:  "Nublado",
		45: "Nevoeiro",
		48: "Nevoeiro com geada",
		51: "Garoa fraca",
		53: "Garoa moderada",
		55: "Garoa intensa",
		56: "Garoa congelante fraca",
		57: "Garoa congelante intensa",
		61: "Chuva fraca",
		63: "Chuva moderada",
		65: "Chuva forte",
		66: "Chuva congelante fraca",
		67: "Chuva congelante forte",
		71: "Neve fraca",
		73: "Neve moderada",
		75: "Neve forte",
		77: "Grãos de neve",
		80: "Pancadas de chuva fracas",
		81: "Pancadas de chuva moderadas",
		82: "Pancadas de chuva violentas",
		85: "Pancadas de neve fracas",
		86: "Pancadas de neve fortes",
		95: "Trovoada fraca ou moderada",
		96: "Trovoada com granizo fraco",
		99: "Trovoada com granizo forte",
	} {
		t.Run(strconv.Itoa(code), func(t *testing.T) {
			t.Parallel()

			if got := weather.ConditionDescription(code); got != want {
				t.Errorf("ConditionDescription(%d) = %q, want %q", code, got, want)
			}
		})
	}
}

func TestConditionDescriptionUnknownCode(t *testing.T) {
	t.Parallel()

	for _, code := range []int{-1, 4, 50, 100} {
		if got := weather.ConditionDescription(code); got != "Condição não informada" {
			t.Errorf("ConditionDescription(%d) = %q, want %q", code, got, "Condição não informada")
		}
	}
}
