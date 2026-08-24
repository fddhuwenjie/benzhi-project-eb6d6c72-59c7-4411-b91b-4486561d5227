package redaction

import "regexp"

func mustPattern(value string) *regexp.Regexp { return regexp.MustCompile(value) }
