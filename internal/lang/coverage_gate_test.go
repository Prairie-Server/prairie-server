package lang

import "testing"

// These cases pin the BCP 47 edge paths Prairie's coverage gate requires:
// extlang/script/region casing, grandfathered forms, duplicate subtags, and
// the verbatim fallbacks for inputs the parser cannot resolve.

func TestCanonicalTagEdgeForms(t *testing.T) {
	cases := map[string]string{
		"zh-cmn-Hans-CN":               "zh-cmn-Hans-CN",
		"zh-yue-hk":                    "zh-yue-HK",
		"ar-eg-u-nu-latn":              "ar-EG-u-nu-latn",
		"x-Private-Thing":              "x-private-thing",
		"en-xyz":                       "en-xyz",
		"sgn-be-fr":                    "sfb",
		"i-klingon":                    "tlh",
		"en-GB-oed":                    "en-GB-oxendict",
		"not a tag":                    "",
		"en-US-u-ca-gregory-u-nu-latn": "",
		"de-1996-1996":                 "",
		"EN_us":                        "en-US",
		"qaa-Latn-QM":                  "qaa-Latn-QM",
		"abc-def-Latn-ZZ-x-ABC":        "abc-def-Latn-ZZ-x-abc",
		"und":                          "und",
	}
	for in, want := range cases {
		if got := CanonicalTag(in); got != want {
			t.Errorf("CanonicalTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalTagCaseRules(t *testing.T) {
	cases := map[string]string{
		"x-foo-BAR":            "x-foo-bar",
		"sr-latn-rs-u-nu-latn": "sr-Latn-RS-u-nu-latn",
		"es-419-VAL1994":       "es-419-val1994",
	}
	for in, want := range cases {
		if got := canonicalTagCase(in); got != want {
			t.Errorf("canonicalTagCase(%q) = %q, want %q", in, got, want)
		}
	}
	if isAlpha("ab1") || !isAlpha("Latn") {
		t.Fatal("isAlpha must accept only ASCII letters")
	}
}

func TestPrimaryLanguageEdgeForms(t *testing.T) {
	cases := map[string]string{
		"x-prairie-original": "",
		"und":                "",
		"":                   "",
		"pt-BR":              "pt",
		"English":            "en",
		"zzz-QQ":             "zzz",
	}
	for in, want := range cases {
		if got := PrimaryLanguage(in); got != want {
			t.Errorf("PrimaryLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalFallsBackVerbatim(t *testing.T) {
	cases := map[string]string{
		"":        "",
		"EN":      "en",
		"fil":     "fil",
		"Klingon": "klingon",
		"zz":      "zz",
		"und":     "und",
		"pt-BR":   "pt",
		"123":     "123",
	}
	for in, want := range cases {
		if got := Canonical(in); got != want {
			t.Errorf("Canonical(%q) = %q, want %q", in, got, want)
		}
	}
}
