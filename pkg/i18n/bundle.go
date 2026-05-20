// Bundle-backed translator for HelixMemory.
//
// Round-359 §11.4 — CONST-046 Phase 4. translator.go ships the Translator
// seam + NoopTranslator fallback; this file adds BundleTranslator, the
// first real (non-noop) translator: it resolves keys against the embedded
// YAML resource files in bundle/*.yaml.
//
// CONST-046 compliance: the format strings live ONLY in the bundle — no
// user-facing English literal appears in any .go source file. Call sites
// pass the stable key + fmt args; BundleTranslator owns the format string.
//
// CONST-051(B) decoupling: the bundle is embedded into HelixMemory itself,
// so the SDK is self-sufficient — a consumer gets working English out of
// the box and MAY still override with its own Translator via Set for
// additional locales.
package i18n

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// bundleFS embeds every locale resource file shipped with HelixMemory.
// Adding a new locale = dropping a sibling <bcp47>.yaml file here.
//
//go:embed bundle/*.yaml
var bundleFS embed.FS

// BundleTranslator resolves keys against the embedded locale bundles.
//
// Lookup order for Translate(locale, key, args...):
//  1. exact locale match (e.g. "sr-Cyrl-RS")
//  2. primary subtag match (e.g. "sr" for "sr-Cyrl-RS")
//  3. the default locale ("en")
//  4. the key verbatim, prefix-stripped (NoopTranslator behaviour)
//
// When a format string is found it is rendered with fmt.Sprintf(args...);
// when args is empty the format string is returned as-is.
type BundleTranslator struct {
	defaultLocale string
	mu            sync.RWMutex
	// locales maps a BCP-47 tag to its key→format-string table.
	locales map[string]map[string]string
}

// NewBundleTranslator loads every bundle/*.yaml file and returns a ready
// translator. defaultLocale is the fallback when a requested locale is
// absent; it MUST itself be present in the bundle or NewBundleTranslator
// returns an error (fail-loud — a translator that silently has no default
// is a CONST-046 trap).
func NewBundleTranslator(defaultLocale string) (*BundleTranslator, error) {
	if defaultLocale == "" {
		defaultLocale = "en"
	}
	entries, err := fs.ReadDir(bundleFS, "bundle")
	if err != nil {
		return nil, fmt.Errorf("i18n: read embedded bundle dir: %w", err)
	}
	bt := &BundleTranslator{
		defaultLocale: defaultLocale,
		locales:       make(map[string]map[string]string),
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		locale := strings.TrimSuffix(name, ".yaml")
		raw, rerr := bundleFS.ReadFile("bundle/" + name)
		if rerr != nil {
			return nil, fmt.Errorf("i18n: read bundle %q: %w", name, rerr)
		}
		table := make(map[string]string)
		if uerr := yaml.Unmarshal(raw, &table); uerr != nil {
			return nil, fmt.Errorf("i18n: parse bundle %q: %w", name, uerr)
		}
		bt.locales[locale] = table
	}
	if _, ok := bt.locales[defaultLocale]; !ok {
		return nil, fmt.Errorf("i18n: default locale %q has no bundle file (have: %v)",
			defaultLocale, bt.localeNames())
	}
	return bt, nil
}

// localeNames returns the sorted-ish set of loaded locale tags (diagnostics).
func (b *BundleTranslator) localeNames() []string {
	names := make([]string, 0, len(b.locales))
	for k := range b.locales {
		names = append(names, k)
	}
	return names
}

// Translate satisfies Translator. It never panics and never returns "".
func (b *BundleTranslator) Translate(locale, key string, args ...interface{}) string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	format, ok := b.lookup(locale, key)
	if !ok {
		// Contract: unknown key returns the key verbatim, prefix-stripped.
		return strings.TrimPrefix(key, BundlePrefix)
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

// lookup walks the locale fallback chain. Caller holds b.mu.RLock.
func (b *BundleTranslator) lookup(locale, key string) (string, bool) {
	tryLocales := make([]string, 0, 3)
	if locale != "" {
		tryLocales = append(tryLocales, locale)
		if i := strings.IndexAny(locale, "-_"); i > 0 {
			tryLocales = append(tryLocales, locale[:i])
		}
	}
	tryLocales = append(tryLocales, b.defaultLocale)

	for _, loc := range tryLocales {
		table, ok := b.locales[loc]
		if !ok {
			continue
		}
		if v, found := table[key]; found {
			return v, true
		}
	}
	return "", false
}
