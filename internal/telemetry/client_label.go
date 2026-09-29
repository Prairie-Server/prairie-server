package telemetry

import "strings"

// ClientLabelName is the metric label that carries a ClientLabel family.
const ClientLabelName = "client"

// The client families, the only values ClientLabel returns.
const (
	clientNone    = "none"
	clientWeb     = "web"
	clientApple   = "apple"
	clientAndroid = "android"
	clientOther   = "other"
)

// ClientLabel maps only recognized first-party product names into the fixed
// client families used as the `client` metric label: web, apple, android,
// other, or none for a nameless client. Arbitrary self-reported names cannot
// create metric series or store private text in Prometheus. Logs retain the
// existing clamped client identity for diagnosis.
//
// First-party apps identify as "Prairie <product>". Builds from before the
// rebrand still send "Silo <product>", so both brand prefixes fold into the
// same family rather than counting a stale app as a third-party client.
func ClientLabel(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return clientNone
	}
	product, ok := strings.CutPrefix(normalized, "prairie ")
	if !ok {
		product, ok = strings.CutPrefix(normalized, "silo ")
	}
	if !ok {
		return clientOther
	}
	switch product {
	case "web":
		return clientWeb
	case "apple", "apple tv", "ios", "tvos", "macos", "ipados":
		return clientApple
	case "android", "android tv":
		return clientAndroid
	default:
		return clientOther
	}
}
