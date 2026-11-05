package zt

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

type apiError struct {
	msg    string
	errors []string
}

func (e *apiError) Error() string { return e.msg }

func apiFail(msg string, errs ...string) error {
	if errs == nil {
		errs = []string{}
	}
	return &apiError{msg: msg, errors: errs}
}

func apiFailf(format string, a ...any) error {
	return apiFail(fmt.Sprintf(format, a...))
}

type apiAction func(p M) (M, error)

var apiActions = map[string]apiAction{}

func registerAction(name string, fn apiAction) {
	apiActions[name] = fn
}

var reAction = regexp.MustCompile(`^[a-z_]{2,40}$`)

func apiDispatch(action string, p M) (out M) {
	fn, ok := apiActions[action]
	if !reAction.MatchString(action) || !ok {
		return M{"ok": false, "error": fmt.Sprintf("Unknown action \"%s\"", action)}
	}
	defer func() {
		if r := recover(); r != nil {
			out = M{"ok": false, "error": fmt.Sprint(r)}
		}
	}()
	result, err := fn(p)
	if err != nil {
		if ae, ok := err.(*apiError); ok {
			return M{"ok": false, "error": ae.msg, "errors": ae.errors}
		}
		return M{"ok": false, "error": err.Error()}
	}
	res := M{"ok": true}
	for k, v := range result {
		if k != "ok" {
			res[k] = v
		}
	}
	return res
}

func readParams(r io.Reader) M {
	b, _ := io.ReadAll(r)
	var p M
	if json.Unmarshal(b, &p) != nil || p == nil {
		p = M{}
	}
	return p
}

func init() {
	register("api", "answer one action of the web interface: api <action>, parameters as JSON on stdin", func(args []string) int {
		action := ""
		if len(args) > 0 {
			action = args[0]
		}
		p := M{}
		if len(args) < 2 {
			p = readParams(os.Stdin)
		} else {
			var q M
			if json.Unmarshal([]byte(args[1]), &q) == nil && q != nil {
				for k, v := range q {
					switch v.(type) {
					case M, []any:
						v = string(jsonEncode(v, false))
						v = strings.TrimRight(v.(string), "\n")
					}
					p[k] = v
				}
			}
		}
		out := apiDispatch(action, p)
		if len(args) > 1 {
			os.Stdout.Write(jsonEncode(out, true))
		} else {
			os.Stdout.Write(jsonEncode(out, false))
		}
		return 0
	})
}

func param(p M, key string, def any) any {
	if v, ok := p[key]; ok {
		return v
	}
	return def
}

func pstr(p M, key string, def string) string {
	v, ok := p[key]
	if !ok || v == nil {
		return def
	}
	return str(v)
}

func pjson(p M, key string) (any, error) {
	v, ok := p[key]
	if !ok || v == nil {
		return nil, apiFailf("Missing or malformed \"%s\"", key)
	}
	switch x := v.(type) {
	case M, []any:
		return x, nil
	case string:
		var d any
		if json.Unmarshal([]byte(x), &d) == nil {
			switch d.(type) {
			case M, []any:
				return d, nil
			}
		}
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}

func pjsonMap(p M, key string) (M, error) {
	v, err := pjson(p, key)
	if err != nil {
		return nil, err
	}
	if m, ok := v.(M); ok {
		return m, nil
	}
	if l, ok := v.([]any); ok && len(l) == 0 {
		return M{}, nil
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}

func pjsonList(p M, key string) ([]any, error) {
	v, err := pjson(p, key)
	if err != nil {
		return nil, err
	}
	if l, ok := v.([]any); ok {
		return l, nil
	}
	if m, ok := v.(M); ok {
		out := []any{}
		for _, x := range m {
			out = append(out, x)
		}
		return out, nil
	}
	return nil, apiFailf("Missing or malformed \"%s\"", key)
}
