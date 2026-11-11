package zt

import (
	"strings"
)

func convNameChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
		c == '_' || c == '.' || c == ':' || c == ' ' || c == '-'
}

var convUmlauts = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss")

func convDatasetName(name string, replaceSpaces bool) string {
	if replaceSpaces {
		name = strings.ReplaceAll(name, " ", "_")
	}
	name = convUmlauts.Replace(name)
	b := []byte(name)
	for i, c := range b {
		if !convNameChar(c) {
			b[i] = '_'
		}
	}
	return string(b)
}

func convSplitExtra(v string) []string {
	out := []string{}
	for _, e := range strings.Split(v, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}
