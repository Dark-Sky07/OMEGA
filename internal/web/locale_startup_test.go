package web

import (
	"testing"

	"github.com/Dark-Sky07/phoenix/internal/web/locale"
)

type translationTestSettings struct{}

func (translationTestSettings) GetTgLang() (string, error) { return "en-US", nil }

func TestEmbeddedTranslationsLoadAtStartup(t *testing.T) {
	if err := locale.InitLocalizer(i18nFS, translationTestSettings{}); err != nil {
		t.Fatalf("embedded translations failed to load: %v", err)
	}
}
