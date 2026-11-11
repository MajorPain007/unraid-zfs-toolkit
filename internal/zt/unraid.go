package zt

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var notifyBin = envOr("ZDC_NOTIFY", "/usr/local/emhttp/webGui/scripts/notify")

var leadingInt = regexp.MustCompile(`^\s*-?\d+`)

func phpInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(leadingInt.FindString(s)))
	return n
}

func fileExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode()&0o111 != 0
}

func notifyArgs(subject, description, level string) []string {
	return []string{notifyBin, "-e", "ZFS Toolkit", "-s", subject, "-d", description, "-i", level}
}

func notify(subject, description, level string) bool {
	if !fileExecutable(notifyBin) {
		return false
	}
	if level == "" {
		level = "normal"
	}
	quoted := make([]string, 0, 10)
	for _, a := range notifyArgs(subject, description, level) {
		quoted = append(quoted, shellQuote(a))
	}
	cmd := exec.Command("/bin/sh", "-c", "nohup setsid "+strings.Join(quoted, " ")+" >/dev/null 2>&1 &")
	cmd.Dir = "/"
	cmd.Run()
	return true
}

func notificationsOn() bool {
	return yes(loadSettings().Get("send_notifications", "yes"))
}
