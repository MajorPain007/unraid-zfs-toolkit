package zt

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	sendSSHBin    = envOr("ZDC_SSH", "ssh")
	sendRemoteZfs = envOr("ZDC_REMOTE_ZFS", "zfs")
	errSendCancel = errors.New("cancelled")
)

func sendExec(ctx context.Context, timeout time.Duration, stdin io.Reader, name string, args ...string) result {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 5 * time.Second
	cmd.Stdin = stdin
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
		if ctx.Err() == context.DeadlineExceeded {
			errb.WriteString("\ntimed out")
		} else if errb.Len() == 0 {
			errb.WriteString(err.Error())
		}
	}
	return result{Code: code, Stdout: out.String(), Stderr: errb.String()}
}

func sendKeyForUse(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", errors.New("ssh key '" + path + "' does not exist")
	}
	if !st.Mode().IsRegular() {
		return "", errors.New("ssh key '" + path + "' is not a file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("ssh key '" + path + "' is not readable")
	}
	if st.Mode().Perm()&0o077 == 0 {
		return path, nil
	}
	sum := sha1.Sum([]byte(path))
	dir := TmpDir + "/ssh"
	os.MkdirAll(dir, 0o700)
	os.Chmod(dir, 0o700)
	dst := filepath.Join(dir, "key-"+hex.EncodeToString(sum[:6]))
	if !fileWrite(dst, data, 0o600) {
		return "", errors.New("cannot prepare a private copy of ssh key '" + path + "'")
	}
	os.Chmod(dst, 0o600)
	return dst, nil
}

func (j sendJob) sshArgs() ([]string, error) {
	a := []string{
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + sendKnownHosts(),
		"-o", "ConnectTimeout=15",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=4",
		"-o", "LogLevel=ERROR",
		"-p", strconv.Itoa(j.SSHPort),
	}
	if j.SSHKey != "" {
		k, err := sendKeyForUse(j.SSHKey)
		if err != nil {
			return nil, err
		}
		a = append(a, "-i", k, "-o", "IdentitiesOnly=yes")
	}
	return append(a, "--", j.SSHHost), nil
}

func (j sendJob) destZfs(ctx context.Context, timeout time.Duration, args ...string) result {
	if !j.remote() {
		return sendExec(ctx, timeout, nil, ZfsBin, args...)
	}
	ssh, err := j.sshArgs()
	if err != nil {
		return result{Code: -1, Stderr: err.Error()}
	}
	return sendExec(ctx, timeout, nil, sendSSHBin, append(ssh, sendRemoteCommand(append([]string{sendRemoteZfs}, args...)...))...)
}

func srcZfs(ctx context.Context, timeout time.Duration, args ...string) result {
	return sendExec(ctx, timeout, nil, ZfsBin, args...)
}

func sendParseSnaps(r result) []sendSnap {
	out := []sendSnap{}
	for _, l := range r.Lines() {
		f := strings.Split(l, "\t")
		if len(f) < 3 {
			continue
		}
		_, snap, ok := strings.Cut(f[0], "@")
		if !ok {
			continue
		}
		out = append(out, sendSnap{Name: snap, GUID: f[1], Creation: toInt64(f[2])})
	}
	return out
}

var sendSnapCols = []string{"list", "-H", "-p", "-t", "snapshot", "-o", "name,guid,creation", "-s", "createtxg", "-d", "1"}

func sendSSHExplain(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "permission denied"):
		return "the server did not accept the key. Add the public key to root's authorized_keys there (see SSH key)."
	case strings.Contains(m, "remote host identification has changed") || strings.Contains(m, "host key verification failed"):
		return "the server's host key is not the one seen before. If the server was reinstalled, forget the old host key and test again."
	case strings.Contains(m, "could not resolve hostname") || strings.Contains(m, "name or service not known"):
		return "the host name cannot be resolved."
	case strings.Contains(m, "connection refused"):
		return "the connection was refused - is SSH running on that port?"
	case strings.Contains(m, "timed out") || strings.Contains(m, "no route to host") || strings.Contains(m, "network is unreachable"):
		return "the server did not answer (timed out / unreachable)."
	case strings.Contains(m, "command not found") || strings.Contains(m, "not found"):
		return "zfs was not found on the server."
	}
	return ""
}

type sendPipeStats struct {
	mu      sync.Mutex
	bytes   int64
	started time.Time
}

func (s *sendPipeStats) add(n int) {
	s.mu.Lock()
	s.bytes += int64(n)
	s.mu.Unlock()
}

func (s *sendPipeStats) get() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytes
}

type limitedBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *limitedBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
		} else {
			b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(b.buf.String())
}

func sendPipeline(ctx context.Context, j sendJob, sendArgs, recvArgs []string, stats *sendPipeStats, tick func()) error {
	sendCmd := exec.Command(ZfsBin, sendArgs...)
	var recvCmd *exec.Cmd
	if j.remote() {
		ssh, err := j.sshArgs()
		if err != nil {
			return err
		}
		recvCmd = exec.Command(sendSSHBin, append(ssh, sendRemoteCommand(append([]string{sendRemoteZfs}, recvArgs...)...))...)
	} else {
		recvCmd = exec.Command(ZfsBin, recvArgs...)
	}
	for _, c := range []*exec.Cmd{sendCmd, recvCmd} {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		c.Dir = "/"
	}
	sendErr := &limitedBuf{max: 8192}
	recvOut := &limitedBuf{max: 8192}
	sendCmd.Stderr = sendErr
	recvCmd.Stdout = recvOut
	recvCmd.Stderr = recvOut
	src, err := sendCmd.StdoutPipe()
	if err != nil {
		return err
	}
	dst, err := recvCmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := recvCmd.Start(); err != nil {
		return errors.New("cannot start zfs recv: " + err.Error())
	}
	if err := sendCmd.Start(); err != nil {
		dst.Close()
		syscall.Kill(-recvCmd.Process.Pid, syscall.SIGKILL)
		recvCmd.Wait()
		return errors.New("cannot start zfs send: " + err.Error())
	}
	killAll := func() {
		syscall.Kill(-sendCmd.Process.Pid, syscall.SIGKILL)
		syscall.Kill(-recvCmd.Process.Pid, syscall.SIGKILL)
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			killAll()
		case <-done:
		}
	}()
	stats.started = time.Now()
	chunks := make(chan []byte, 16)
	pool := sync.Pool{New: func() any { return make([]byte, 1<<20) }}
	go func() {
		defer close(chunks)
		for {
			b := pool.Get().([]byte)
			n, err := io.ReadFull(src, b)
			if n > 0 {
				chunks <- b[:n]
			}
			if err != nil {
				return
			}
		}
	}()
	var writeErr error
	lastTick := time.Now()
	limit := float64(j.BwLimit) * 1024 * 1024
	for c := range chunks {
		if writeErr == nil {
			if _, err := dst.Write(c); err != nil {
				writeErr = err
				syscall.Kill(-sendCmd.Process.Pid, syscall.SIGTERM)
			} else {
				stats.add(len(c))
			}
		}
		pool.Put(c[:cap(c)])
		if limit > 0 && writeErr == nil {
			want := time.Duration(float64(stats.get()) / limit * float64(time.Second))
			if ahead := want - time.Since(stats.started); ahead > 0 {
				select {
				case <-time.After(ahead):
				case <-ctx.Done():
				}
			}
		}
		if time.Since(lastTick) > 2*time.Second {
			lastTick = time.Now()
			tick()
		}
	}
	dst.Close()
	sendWait := sendCmd.Wait()
	recvWait := recvCmd.Wait()
	close(done)
	if ctx.Err() != nil {
		return errSendCancel
	}
	if sendWait == nil && recvWait == nil {
		return nil
	}
	parts := []string{}
	if sendWait != nil {
		msg := sendErr.String()
		if msg == "" {
			msg = sendWait.Error()
		}
		parts = append(parts, "zfs send: "+sendLastLines(msg, 6))
	}
	if recvWait != nil {
		msg := recvOut.String()
		if msg == "" {
			msg = recvWait.Error()
		}
		if strings.Contains(msg, "has been modified") || strings.Contains(msg, "does not match incremental source") || strings.Contains(msg, "destination already exists") {
			msg += " - the destination changed since the last run; turn on Force once to roll it back"
		}
		label := "zfs recv: "
		if j.remote() {
			if x := sendSSHExplain(msg); x != "" && !strings.Contains(msg, "cannot receive") {
				msg += " (" + x + ")"
			}
			label = "zfs recv on " + j.SSHHost + ": "
		}
		parts = append(parts, label+sendLastLines(msg, 6))
	}
	return errors.New(strings.Join(parts, "; "))
}

func sendLastLines(s string, n int) string {
	l := strings.Split(strings.TrimSpace(s), "\n")
	if len(l) > n {
		l = l[len(l)-n:]
	}
	return strings.Join(l, " / ")
}
