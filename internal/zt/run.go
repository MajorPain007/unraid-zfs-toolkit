package zt

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

type result struct {
	Code   int
	Stdout string
	Stderr string
}

func (r result) OK() bool { return r.Code == 0 }

func (r result) Lines() []string {
	out := strings.TrimRight(r.Stdout, "\n")
	if out == "" {
		return []string{}
	}
	return strings.Split(out, "\n")
}

func (r result) Message() string {
	msg := strings.TrimSpace(r.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(r.Stdout)
	}
	if msg == "" {
		return "no error output"
	}
	return msg
}

func runCmd(timeout time.Duration, name string, args ...string) result {
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
		if errb.Len() == 0 {
			errb.WriteString(err.Error())
		}
	}
	return result{Code: code, Stdout: out.String(), Stderr: errb.String()}
}

func zfs(args ...string) result {
	return runCmd(10*time.Minute, ZfsBin, args...)
}

func zpool(args ...string) result {
	return runCmd(2*time.Minute, ZpoolBin, args...)
}
