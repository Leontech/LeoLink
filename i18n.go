package main

// Language. LeoLink is written in Czech; English texts come from enCatalog (i18n_en.go), keyed by the Czech text
// (for messages with values: the Czech format string). The language also names the entities in Home Assistant.

import (
	"os"
	"strings"
	"sync/atomic"
)

var curLang atomic.Value // "cs" | "en"

// resolveLang turns the setting ("" = automatic, by the Windows display language) into the language used.
func resolveLang(setting string) string {
	switch setting {
	case "cs", "en":
		return setting
	}
	if l := os.Getenv("LEOLINK_LANG"); l == "cs" || l == "en" {
		return l
	}
	if osLangCzech() {
		return "cs"
	}
	return "en"
}

func setLang(setting string) { curLang.Store(resolveLang(setting)) }

func lang() string {
	if l, ok := curLang.Load().(string); ok {
		return l
	}
	return "cs"
}

// T translates a Czech text / format string to the current language (missing translation → Czech).
func T(cz string) string {
	if lang() == "en" {
		if en, ok := enCatalog[cz]; ok {
			return en
		}
	}
	return cz
}

func envLangCzech() bool {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.ToLower(os.Getenv(k)); v != "" {
			return strings.HasPrefix(v, "cs") || strings.HasPrefix(v, "sk")
		}
	}
	return false
}
