package otlp

import (
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

// Free-text scrubbing masks credentials that appear inside a string value, such
// as a log body, an exception message or a URL. Attribute names are handled by
// normalizer.redacts; this covers values whose key is innocent.
//
// The rules favour precision: each one needs a recognisable shape (a scheme, a
// prefix, or a sensitive name followed by ':' or '='). A prose sentence like
// "the password is wrong" is left alone, so a secret written as plain prose
// still needs filtering in the SDK or Collector.
var (
	privateKeyBlock = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	// name=value, name: value and "name":"value", where the name ends in a
	// sensitive word. A value may carry a Bearer or Basic scheme.
	sensitivePair = regexp.MustCompile(`(?i)("?[a-z0-9_.-]*(?:password|passwd|pwd|secret|token|api[_.-]?key|apikey|authorization|credential|private[_.-]?key|access[_.-]?key|cookie)"?\s*[:=]\s*)((?:(?:bearer|basic)\s+)?(?:\[REDACTED\]|"[^"]*"|'[^']*'|[^\s,;&"'}\]]+))`)
	authScheme    = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{8,}`)
	jsonWebToken  = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)
	urlUserinfo   = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)[^\s/@:]+:[^\s/@]+@`)
	// Provider tokens with a fixed prefix: AWS access key IDs, GitHub, Slack,
	// OpenAI-style keys and TracePilot's own API keys.
	knownToken = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b|\bgh[pousr]_[A-Za-z0-9]{36,}|\bxox[abprs]-[A-Za-z0-9-]{10,}|\bsk-[A-Za-z0-9_-]{20,}|\btracepilot_[A-Za-z0-9_-]{20,}`)
)

// Each rule has cheap substring triggers, matched against the lower-cased
// value, so a string runs only the expressions it could match. Most values
// match none and skip every expression.
var (
	pairWords    = []string{"password", "passwd", "pwd", "secret", "token", "apikey", "api_key", "api-key", "api.key", "authorization", "credential", "private_key", "private-key", "private.key", "access_key", "access-key", "access.key", "cookie"}
	tokenPrefixs = []string{"akia", "asia", "ghp_", "gho_", "ghu_", "ghs_", "ghr_", "xox", "sk-", "tracepilot_"}
)

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func scrubText(value string) string {
	if len(value) < 8 {
		return value
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "-----begin") {
		value = privateKeyBlock.ReplaceAllString(value, redacted)
	}
	if strings.ContainsAny(lower, ":=") && containsAny(lower, pairWords) {
		value = sensitivePair.ReplaceAllStringFunc(value, maskPair)
	}
	if strings.Contains(lower, "://") && strings.Contains(lower, "@") {
		value = urlUserinfo.ReplaceAllString(value, "${1}"+redacted+"@")
	}
	if strings.Contains(lower, "bearer") || strings.Contains(lower, "basic") {
		value = authScheme.ReplaceAllString(value, "${1} "+redacted)
	}
	if strings.Contains(lower, "eyj") {
		value = jsonWebToken.ReplaceAllString(value, redacted)
	}
	if containsAny(lower, tokenPrefixs) {
		value = knownToken.ReplaceAllString(value, redacted)
	}
	return value
}

// scrub applies free-text scrubbing when the deployment has it enabled.
func (normalizer *normalizer) scrub(value string) string {
	if !normalizer.configuration.ScrubFreeText {
		return value
	}
	return scrubText(value)
}

// maskPair keeps the name and the quote style of a name=value pair and hides the value.
func maskPair(match string) string {
	parts := sensitivePair.FindStringSubmatch(match)
	quote := ""
	if value := parts[2]; value != "" && (value[0] == '"' || value[0] == '\'') {
		quote = value[:1]
	}
	return parts[1] + quote + redacted + quote
}
