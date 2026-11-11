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

func TestToBool(t *testing.T) {
	cases := []struct {
		v    any
		def  bool
		want bool
	}{
		{true, false, true}, {false, true, false},
		{1.0, false, true}, {2.0, false, true}, {-1.0, false, true}, {0.0, true, false},
		{1, false, true}, {0, true, false},
		{"1", false, true}, {"2", false, true}, {"0", true, false},
		{"true", false, true}, {"TRUE", false, true}, {" yes ", false, true}, {"On", false, true},
		{"false", true, false}, {"no", true, false}, {"off", true, false}, {"", true, false}, {"x", true, false},
		{nil, true, true}, {nil, false, false}, {M{}, true, true}, {[]any{}, false, false},
	}
	for _, c := range cases {
		if got := toBool(c.v, c.def); got != c.want {
			t.Errorf("toBool(%#v, %v) = %v, want %v", c.v, c.def, got, c.want)
		}
	}
}

func TestSendJobBoolDefaults(t *testing.T) {
	j := sendJobFromMap(M{"id": "a", "enabled": nil}, 0)
	if !j.Enabled || !j.Compressed || j.Recursive || j.Raw || j.AllowRollback || j.Readonly {
		t.Errorf("defaults: %+v", j)
	}
	j = sendJobFromMap(M{"id": "a", "enabled": "0", "compressed": "no", "recursive": 1.0, "raw": "TRUE", "allow_rollback": "on", "readonly": true}, 0)
	if j.Enabled || j.Compressed || !j.Recursive || !j.Raw || !j.AllowRollback || !j.Readonly {
		t.Errorf("values: %+v", j)
	}
}
