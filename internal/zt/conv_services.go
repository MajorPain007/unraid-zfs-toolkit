package zt

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type convService struct {
	Kind    string   `json:"kind"`
	Name    string   `json:"name"`
	ID      string   `json:"id"`
	Running bool     `json:"running"`
	Paths   []string `json:"paths"`
}

func convHaveBin(bin string) bool {
	if strings.Contains(bin, "/") {
		return fileExecutable(bin)
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

func convResolveUserPath(p string) []string {
	p = filepath.Clean(p)
	if !strings.HasPrefix(p, "/mnt/user/") {
		return []string{p}
	}
	rel := strings.TrimPrefix(p, "/mnt/user")
	out := []string{}
	des, _ := os.ReadDir("/mnt")
	for _, d := range des {
		n := d.Name()
		if n == "user" || n == "user0" || !d.IsDir() {
			continue
		}
		cand := "/mnt/" + n + rel
		if exists(cand) {
			out = append(out, cand)
		}
	}
	return out
}

type convDockerInspect struct {
	ID     string                 `json:"Id"`
	Name   string                 `json:"Name"`
	State  struct{ Running bool } `json:"State"`
	Mounts []struct {
		Type   string `json:"Type"`
		Source string `json:"Source"`
	} `json:"Mounts"`
}

func convContainers(all bool) ([]convService, string) {
	if !convHaveBin(convDockerBin) {
		return nil, "docker command not found, skipping container handling."
	}
	args := []string{"ps", "-q", "--no-trunc"}
	if all {
		args = append(args, "-a")
	}
	r := runCmd(30*time.Second, convDockerBin, args...)
	if !r.OK() {
		return nil, "docker ps failed: " + r.Message()
	}
	ids := []string{}
	for _, l := range r.Lines() {
		if l = strings.TrimSpace(l); l != "" {
			ids = append(ids, l)
		}
	}
	if len(ids) == 0 {
		return []convService{}, ""
	}
	r = runCmd(60*time.Second, convDockerBin, append([]string{"inspect"}, ids...)...)
	if !r.OK() {
		return nil, "docker inspect failed: " + r.Message()
	}
	var raw []convDockerInspect
	if err := json.Unmarshal([]byte(r.Stdout), &raw); err != nil {
		return nil, "docker inspect gave unreadable output: " + err.Error()
	}
	out := []convService{}
	for _, c := range raw {
		s := convService{Kind: "container", Name: strings.TrimPrefix(c.Name, "/"), ID: c.ID, Running: c.State.Running, Paths: []string{}}
		for _, m := range c.Mounts {
			if m.Type == "bind" && m.Source != "" {
				s.Paths = append(s.Paths, convResolveUserPath(m.Source)...)
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return natLess(out[i].Name, out[j].Name) })
	return out, ""
}

func convVMDisks(vm string) []string {
	r := runCmd(30*time.Second, convVirshBin, "domblklist", vm, "--details")
	out := []string{}
	if !r.OK() {
		return out
	}
	for _, l := range r.Lines() {
		f := strings.Fields(l)
		if len(f) < 4 || f[0] == "Type" || strings.HasPrefix(f[0], "-") {
			continue
		}
		if f[1] != "disk" && f[1] != "cdrom" {
			continue
		}
		idx := strings.Index(l, f[2])
		src := strings.TrimSpace(l[idx+len(f[2]):])
		if strings.HasPrefix(src, "/") {
			out = append(out, convResolveUserPath(src)...)
		}
	}
	return out
}

func convVMs(all bool) ([]convService, string) {
	if !convHaveBin(convVirshBin) {
		return nil, "virsh command not found, skipping VM handling."
	}
	args := []string{"list", "--name"}
	if all {
		args = append(args, "--all")
	}
	r := runCmd(30*time.Second, convVirshBin, args...)
	if !r.OK() {
		return nil, "virsh list failed: " + r.Message()
	}
	running := map[string]bool{}
	if all {
		rr := runCmd(30*time.Second, convVirshBin, "list", "--name")
		for _, l := range rr.Lines() {
			running[strings.TrimSpace(l)] = true
		}
	}
	out := []convService{}
	for _, l := range r.Lines() {
		vm := strings.TrimSpace(l)
		if vm == "" {
			continue
		}
		out = append(out, convService{Kind: "vm", Name: vm, ID: vm, Running: !all || running[vm], Paths: convVMDisks(vm)})
	}
	sort.Slice(out, func(i, j int) bool { return natLess(out[i].Name, out[j].Name) })
	return out, ""
}

func convVMRunning(vm string) bool {
	r := runCmd(30*time.Second, convVirshBin, "domstate", vm)
	return r.OK() && strings.Contains(strings.ToLower(r.Stdout), "running")
}

func convUsesFolders(s convService, mps []string, folder string) bool {
	for _, p := range s.Paths {
		for _, mp := range mps {
			if convOwnerFolder(filepath.Clean(p), mp) == folder {
				return true
			}
		}
	}
	return false
}

func convAttachUsers(scans []convSourceScan, services []convService) {
	for si := range scans {
		mp := scans[si].Mountpoint
		if mp == "" {
			continue
		}
		for fi := range scans[si].Folders {
			f := &scans[si].Folders[fi]
			for _, s := range services {
				if convUsesFolders(s, []string{mp}, f.Path) {
					f.Users = append(f.Users, convUser{Kind: s.Kind, Name: s.Name, Running: s.Running})
				}
			}
		}
	}
}

func convSleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
