package zt

import (
	"strings"
)

func convNameChar(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' ||
		c == '_' || c == '.' || c == ':' || c == ' ' || c == '-'
}

func convValidComponent(name string) bool {
	if name == "" || name == "." || name == ".." || len(name) > 200 {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !convNameChar(name[i]) {
			return false
		}
	}
	return true
}

var convUmlauts = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "Ä", "Ae", "Ö", "Oe", "Ü", "Ue", "ß", "ss")

func convDatasetName(name string, replaceSpaces bool) string {
	if replaceSpaces {
		name = strings.ReplaceAll(name, " ", "_")
	}
	if convValidComponent(name) {
		return name
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

func convValidDataset(ds string) bool {
	if ds == "" || strings.HasPrefix(ds, "/") || strings.HasSuffix(ds, "/") || len(ds) > 255 {
		return false
	}
	parts := strings.Split(ds, "/")
	for i, p := range parts {
		if !convValidComponent(p) {
			return false
		}
		if i == 0 {
			c := p[0]
			if strings.Contains(p, " ") || !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
				return false
			}
		}
	}
	return true
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
