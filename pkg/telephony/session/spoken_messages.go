package session

import (
	"strings"

	"github.com/ilhasoft/wwcs/config"
)

var spokenCatalog = map[string]map[string]string{
	"voice.greeting": {
		"en":    "The voice assistant is ready",
		"pt":    "O assistente de voz está pronto",
		"pt-br": "O assistente de voz está pronto",
		"es":    "El asistente de voz está listo",
		"ro":    "Asistentul vocal este pregătit",
	},
	"voice.error.stt_unavailable": {
		"en":    "The voice service couldn't start due to a technical issue",
		"pt":    "Não foi possível iniciar o serviço de voz devido a um problema técnico",
		"pt-br": "Não foi possível iniciar o serviço de voz devido a um problema técnico",
		"es":    "No se pudo iniciar el servicio de voz debido a un problema técnico",
		"ro":    "Nu s-a putut porni serviciul vocal din cauza unei probleme tehnice",
	},
	"voice.error.channel_unresolved": {
		"en":    "The voice service couldn't start due to a configuration issue",
		"pt":    "Não foi possível iniciar o serviço de voz devido a um problema de configuração",
		"pt-br": "Não foi possível iniciar o serviço de voz devido a um problema de configuração",
		"es":    "No se pudo iniciar el servicio de voz debido a un problema de configuración",
		"ro":    "Nu s-a putut porni serviciul vocal din cauza unei probleme de configurare",
	},
}

// ResolveSpokenText returns localized spoken copy for the given key and language.
func ResolveSpokenText(key, language string) string {
	lang := normalizeLanguage(language)
	if texts, ok := spokenCatalog[key]; ok {
		if text, ok := texts[lang]; ok {
			return text
		}
		if text, ok := texts["en"]; ok {
			return text
		}
	}
	return ""
}

// ResolveGreetingText returns the configured greeting for the active language.
func ResolveGreetingText(language string) string {
	key := config.Get().Telephony.GreetingTextKey
	if key == "" {
		key = "voice.greeting"
	}
	text := ResolveSpokenText(key, language)
	if text == "" {
		return ResolveSpokenText("voice.greeting", "en")
	}
	return text
}

func normalizeLanguage(language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	lang = strings.ReplaceAll(lang, "_", "-")
	return lang
}
