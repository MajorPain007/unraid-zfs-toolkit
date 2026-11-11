package zt

import (
	"strings"
	"testing"
)

func TestValidDataset(t *testing.T) {
	cases := map[string]bool{
		"cache":                             true,
		"cache/appdata":                     true,
		"cache/TV Shows":                    true,
		"tank/media/photos":                 true,
		"cache/_x":                          true,
		"cache/.x":                          true,
		"cache/_x/.y":                       true,
		"a:b/c-d.e":                         true,
		"cache/-x":                          true,
		"cache/ ":                           true,
		"cache/" + strings.Repeat("a", 201): true,
		"cache/" + strings.Repeat("a", 249): true,
		"cache/" + strings.Repeat("a", 250): false,
		"_x":                                false,
		".x":                                false,
		"-x":                                false,
		"-oProxyCommand=x":                  false,
		"_pool/x":                           false,
		"TV Shows/x":                        false,
		" cache":                            false,
		"cache/Büro":                        false,
		"cache/(x":                          false,
		"cache/a(b)":                        false,
		"cache/a+b":                         false,
		"cache/a,b":                         false,
		"cache/50%":                         false,
		"cache/a;rm":                        false,
		"cache/app@data":                    false,
		"cache@snap":                        false,
		"cache/x\n":                         false,
		"cache/x\t":                         false,
		"":                                  false,
		"/cache":                            false,
		"cache/":                            false,
		"cache//x":                          false,
		"cache/.":                           false,
		"cache/..":                          false,
		"cache/../x":                        false,
	}
	for name, want := range cases {
		if got := validDataset(name); got != want {
			t.Errorf("validDataset(%q) = %v, want %v", name, got, want)
		}
	}
}
