package zt

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var snapTypes = []string{"frequent", "hourly", "daily", "weekly", "monthly", "yearly"}

var reSnapName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.: -]*$`)

func snapValidSnapshot(full string) bool {
	i := strings.Index(full, "@")
	if i <= 0 {
		return false
	}
	return validDataset(full[:i]) && reSnapName.MatchString(full[i+1:]) && len(full) <= 255
}

func snapDatasetsFile() string {
	return ConfigDir + "/snap_datasets.json"
}

type snapDS struct {
	Name        string  `json:"name"`
	Recursive   bool    `json:"recursive"`
	UseTemplate bool    `json:"use_template"`
	Hourly      *string `json:"hourly,omitempty"`
	Daily       *string `json:"daily,omitempty"`
	Weekly      *string `json:"weekly,omitempty"`
	Monthly     *string `json:"monthly,omitempty"`
	Yearly      *string `json:"yearly,omitempty"`
	Frequent    *string `json:"frequent,omitempty"`
}

func (d *snapDS) field(typ string) **string {
	switch typ {
	case "hourly":
		return &d.Hourly
	case "daily":
		return &d.Daily
	case "weekly":
		return &d.Weekly
	case "monthly":
		return &d.Monthly
	case "yearly":
		return &d.Yearly
	case "frequent":
		return &d.Frequent
	}
	return nil
}

func snapRetentionValue(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimSpace(x), true
	case bool:
		if x {
			return "1", true
		}
		return "", true
	}
	s, ok := scalarString(v)
	return strings.TrimSpace(s), ok
}

func snapDSFromMap(m M) snapDS {
	d := snapDS{Name: strings.TrimSpace(str(m["name"])), UseTemplate: true}
	d.Recursive = toBool(m["recursive"])
	if v, ok := m["use_template"]; ok && v != nil {
		d.UseTemplate = toBool(v)
	}
	for _, t := range snapTypes {
		if v, ok := m[t]; ok {
			if s, ok := snapRetentionValue(v); ok {
				val := s
				*d.field(t) = &val
			}
		}
	}
	return d
}

func snapParseDatasets(data []byte) ([]snapDS, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("snap_datasets.json is not valid JSON: %v", err)
	}
	var items []any
	switch x := raw.(type) {
	case M:
		l, _ := x["datasets"].([]any)
		items = l
	case []any:
		items = x
	}
	out := []snapDS{}
	for _, it := range items {
		m, ok := it.(M)
		if !ok {
			continue
		}
		d := snapDSFromMap(m)
		if d.Name == "" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

func snapLoadDatasets() ([]snapDS, bool, error) {
	b, err := os.ReadFile(snapDatasetsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return []snapDS{}, false, nil
		}
		return nil, false, err
	}
	l, err := snapParseDatasets(b)
	return l, true, err
}

func snapSaveDatasets(l []snapDS) error {
	if l == nil {
		l = []snapDS{}
	}
	if !fileWrite(snapDatasetsFile(), jsonEncode(M{"datasets": l}, true), 0o644) {
		return apiFailf("Cannot write %s", snapDatasetsFile())
	}
	return nil
}

func snapGlobalKeep(s Settings, typ string) (int, string) {
	v := strings.TrimSpace(s.Get("snap_"+typ, ""))
	if v == "" {
		return 0, ""
	}
	if !isDigits(v) {
		return 0, fmt.Sprintf("Invalid value for snap_%s='%s', using 0.", typ, v)
	}
	return digitsInt(v), ""
}

func snapGlobalAge(s Settings, typ string) (int, string) {
	v := strings.TrimSpace(s.Get("snap_age_"+typ, ""))
	if v == "" {
		return 0, ""
	}
	if !isDigits(v) {
		return 0, fmt.Sprintf("Invalid value for snap_age_%s='%s', using 0.", typ, v)
	}
	return digitsInt(v), ""
}

func (d snapDS) keep(s Settings, typ string) (int, string) {
	global, warn := snapGlobalKeep(s, typ)
	if d.UseTemplate {
		return global, warn
	}
	p := *d.field(typ)
	if p == nil {
		if typ == "frequent" {
			return 0, ""
		}
		return global, warn
	}
	v := *p
	if v == "" {
		return 0, ""
	}
	if isDigits(v) {
		return digitsInt(v), ""
	}
	return 0, fmt.Sprintf("Ignoring invalid retention '%s' for %s %s - treating it as off", v, d.Name, typ)
}

func (d snapDS) keeps(s Settings) (map[string]int, []string) {
	out := map[string]int{}
	warns := []string{}
	for _, t := range snapTypes {
		n, w := d.keep(s, t)
		out[t] = n
		if w != "" {
			warns = append(warns, w)
		}
	}
	return out, warns
}

func snapDailyHour(s Settings) int {
	return clampInt(s, "snap_daily_hour", 0, 23, 2)
}

func snapMinFreePct(s Settings) int {
	return clampInt(s, "snap_min_free_pct", 0, 90, 5)
}

var reSnapTarget = regexp.MustCompile(`^([0-9]+)([KMGTP]?)I?B?$`)

func snapTargetBytes(spec string, total int64) (int64, bool) {
	spec = strings.ToUpper(strings.Join(strings.Fields(spec), ""))
	if spec == "" {
		return 0, false
	}
	if strings.HasSuffix(spec, "%") {
		n := strings.TrimSuffix(spec, "%")
		if !isDigits(n) || total <= 0 {
			return 0, false
		}
		return total * int64(digitsInt(n)) / 100, true
	}
	m := reSnapTarget.FindStringSubmatch(spec)
	if m == nil || len(m[1]) > 15 {
		return 0, false
	}
	n := toInt64(m[1])
	mult := map[string]int64{"": 1, "K": 1 << 10, "M": 1 << 20, "G": 1 << 30, "T": 1 << 40, "P": 1 << 50}[m[2]]
	return n * mult, true
}

func snapPoolOf(name string) string {
	if i := strings.IndexAny(name, "/@"); i >= 0 {
		return name[:i]
	}
	return name
}

func snapTypeOf(snap string) string {
	if !strings.HasPrefix(snap, "auto-") {
		return ""
	}
	rest := snap[5:]
	i := strings.Index(rest, "-")
	if i <= 0 {
		return ""
	}
	return rest[:i]
}

func snapSortedNames(l []snapDS) []snapDS {
	out := append([]snapDS{}, l...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
