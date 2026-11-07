package zt

import (
	"os"
	"strings"
)

type convJournal struct {
	Source    string `json:"source"`
	Entry     string `json:"entry"`
	Name      string `json:"name"`
	Temp      string `json:"temp"`
	Committed bool   `json:"committed"`
}

func (j convJournal) path() string {
	if strings.HasSuffix(j.Temp, "_temp") {
		p := strings.TrimSuffix(j.Temp, "_temp")
		if convBase(p) == j.Entry {
			return p
		}
	}
	return "/mnt/" + j.Source + "/" + j.Entry
}

func (j convJournal) dataset() string {
	return j.Source + "/" + j.Name
}

func convParseJournal(data string) (convJournal, bool) {
	lines := strings.Split(strings.ReplaceAll(data, "\r", ""), "\n")
	for len(lines) < 5 {
		lines = append(lines, "")
	}
	j := convJournal{Source: lines[0], Entry: lines[1], Name: lines[2], Temp: lines[3], Committed: lines[4] == "committed"}
	if j.Source == "" && j.Entry == "" && j.Temp == "" {
		return j, false
	}
	return j, true
}

func (j convJournal) render() string {
	s := j.Source + "\n" + j.Entry + "\n" + j.Name + "\n" + j.Temp + "\n"
	if j.Committed {
		s += "committed\n"
	}
	return s
}

func convReadJournal() (convJournal, bool) {
	b, err := os.ReadFile(convJournalPath)
	if err != nil {
		return convJournal{}, false
	}
	j, ok := convParseJournal(string(b))
	if !ok {
		return convJournal{Entry: "(unreadable journal)"}, true
	}
	return j, true
}

func convWriteJournal(j convJournal) bool {
	os.MkdirAll(dirOf(convJournalPath), 0o755)
	f, err := os.OpenFile(convJournalPath+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return false
	}
	_, werr := f.WriteString(j.render())
	serr := f.Sync()
	f.Close()
	if werr != nil || serr != nil {
		os.Remove(convJournalPath + ".tmp")
		return false
	}
	if os.Rename(convJournalPath+".tmp", convJournalPath) != nil {
		return false
	}
	if d, err := os.Open(dirOf(convJournalPath)); err == nil {
		d.Sync()
		d.Close()
	}
	return true
}

func convClearJournal() {
	os.Remove(convJournalPath)
}
