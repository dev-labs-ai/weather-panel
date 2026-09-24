package weather

// unknownCondition describes a weather code missing from conditionDescriptions. An unknown code never fails a lookup.
const unknownCondition = "Condição não informada"

// conditionDescriptions maps the WMO weather codes Open-Meteo reports to Brazilian Portuguese descriptions.
var conditionDescriptions = map[int]string{
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
}

// ConditionDescription describes a WMO weather code in Brazilian Portuguese.
func ConditionDescription(code int) string {
	if d, ok := conditionDescriptions[code]; ok {
		return d
	}
	return unknownCondition
}
