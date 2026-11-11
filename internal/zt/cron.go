package zt

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
)

type cronExpr struct {
	sets    [5][64]bool
	domStar bool
	dowStar bool
}

func cronParse(expr string) (*cronExpr, error) {
	if p := validCronProblem(expr); p != "" {
		return nil, errors.New(p)
	}
	c := &cronExpr{}
	parts := strings.Fields(expr)
	for i, part := range parts {
		c.sets[i] = cronParseField(part, i)
	}
	if c.sets[4][7] {
		c.sets[4][0] = true
	}
	c.domStar = parts[2][0] == '*'
	c.dowStar = parts[4][0] == '*'
	return c, nil
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func digitsInt(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return math.MaxInt
	}
	return n
}

func cronParseField(text string, index int) [64]bool {
	var set [64]bool
	for _, item := range strings.Split(text, ",") {
		step := 1
		if a, b, ok := strings.Cut(item, "/"); ok {
			item, step = a, digitsInt(b)
		}
		lo, hi := cronLo[index], cronHi[index]
		if item != "*" {
			a, b, isRange := strings.Cut(item, "-")
			lo, hi = digitsInt(a), digitsInt(a)
			if isRange {
				hi = digitsInt(b)
			} else if step > 1 {
				hi = cronHi[index]
			}
		}
		for v := lo; v <= hi; v += step {
			set[v] = true
		}
	}
	return set
}

func cronDaysIn(y, m int) int {
	return time.Date(y, time.Month(m)+1, 0, 12, 0, 0, 0, time.UTC).Day()
}

func cronLocal(y, m, d, h, i int) int64 {
	return time.Date(y, time.Month(m), d, h, i, 0, 0, time.Local).Unix()
}

func cronFieldsOf(ts int64) (y, m, d, h, i int) {
	t := time.Unix(ts, 0).In(time.Local)
	return t.Year(), int(t.Month()), t.Day(), t.Hour(), t.Minute()
}

func (c *cronExpr) dayMatches(y, m, d int) bool {
	if !c.sets[3][m] {
		return false
	}
	dom := c.sets[2][d]
	dow := c.sets[4][int(time.Date(y, time.Month(m), d, 12, 0, 0, 0, time.UTC).Weekday())]
	if c.domStar || c.dowStar {
		return dom && dow
	}
	return dom || dow
}

func (c *cronExpr) prev(ts int64) (int64, bool) {
	y, m, d, h, i := cronFieldsOf(ts)
	for day := 0; day < 366*28; day++ {
		if c.dayMatches(y, m, d) {
			hh := 23
			if day == 0 {
				hh = h
			}
			for ; hh >= 0; hh-- {
				if !c.sets[1][hh] {
					continue
				}
				mm := 59
				if day == 0 && hh == h {
					mm = i
				}
				for ; mm >= 0; mm-- {
					if c.sets[0][mm] {
						if t := cronLocal(y, m, d, hh, mm); t <= ts {
							return t, true
						}
					}
				}
			}
		}
		if d--; d < 1 {
			if m--; m < 1 {
				m = 12
				y--
			}
			d = cronDaysIn(y, m)
		}
	}
	return 0, false
}

func (c *cronExpr) next(ts int64) (int64, bool) {
	y, m, d, h, i := cronFieldsOf(ts)
	for day := 0; day < 366*28; day++ {
		if c.dayMatches(y, m, d) {
			hh := 0
			if day == 0 {
				hh = h
			}
			for ; hh <= 23; hh++ {
				if !c.sets[1][hh] {
					continue
				}
				mm := 0
				if day == 0 && hh == h {
					mm = i + 1
				}
				for ; mm <= 59; mm++ {
					if c.sets[0][mm] {
						if t := cronLocal(y, m, d, hh, mm); t > ts {
							return t, true
						}
					}
				}
			}
		}
		if d++; d > cronDaysIn(y, m) {
			d = 1
			if m++; m > 12 {
				m = 1
				y++
			}
		}
	}
	return 0, false
}

func (c *cronExpr) upcoming(ts int64, n int) []int64 {
	out := []int64{}
	for len(out) < n {
		t, ok := c.next(ts)
		if !ok {
			break
		}
		out = append(out, t)
		ts = t
	}
	return out
}
