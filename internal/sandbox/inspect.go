package sandbox

import (
	"encoding/json"
	"reflect"
	"strconv"
)

func validateContainer(data []byte, p Profile, user, owner, source, bundle, guard string) error {
	var c struct {
		Image  string
		Config struct {
			Image, User, WorkingDir string
			Entrypoint, Cmd, Env    []string
			Labels                  map[string]string
			Tty, OpenStdin          bool
		}
		HostConfig struct {
			NetworkMode, IpcMode, PidMode           string
			ReadonlyRootfs, Privileged              bool
			CapDrop, CapAdd, SecurityOpt            []string
			Memory, MemorySwap, NanoCpus, PidsLimit int64
			Tmpfs                                   map[string]string
			Devices                                 []any
			Binds                                   []string
			VolumesFrom                             []string
			RestartPolicy                           struct{ Name string }
			LogConfig                               containerLogConfig
		}
		Mounts []struct {
			Type, Source, Destination, Propagation string
			RW                                     bool
		}
	}
	if json.Unmarshal(data, &c) != nil {
		return ErrPolicy
	}
	h := c.HostConfig
	if c.Image != p.Image || c.Config.User != user || c.Config.WorkingDir != "/workspace" || c.Config.Labels["engineering-platform.offline"] != owner || c.Config.Tty || c.Config.OpenStdin || !reflect.DeepEqual(c.Config.Entrypoint, []string{"/ep-guard"}) || !reflect.DeepEqual(c.Config.Cmd, append([]string{strconv.Itoa(p.Seconds)}, p.Argv...)) {
		return ErrPolicy
	}
	if h.NetworkMode != "none" || h.IpcMode != "private" || h.PidMode != "" || !h.ReadonlyRootfs || h.Privileged || len(h.CapAdd) != 0 || len(h.CapDrop) != 1 || h.CapDrop[0] != "ALL" || len(h.SecurityOpt) != 1 || h.SecurityOpt[0] != "no-new-privileges:true" || h.Memory != 256<<20 || h.MemorySwap != 256<<20 || h.NanoCpus != 1_000_000_000 || h.PidsLimit != 64 || h.RestartPolicy.Name != "no" || h.LogConfig.Type != "local" || !reflect.DeepEqual(h.LogConfig.Config, map[string]string{"max-size": "1m", "max-file": "1", "compress": "false"}) || len(h.Devices) != 0 || len(h.Binds) != 0 || len(h.VolumesFrom) != 0 {
		return ErrPolicy
	}
	if len(c.Config.Env) != 4 {
		return ErrPolicy
	}
	env := map[string]bool{}
	for _, v := range c.Config.Env {
		env[v] = true
	}
	for _, v := range []string{"PATH=/usr/bin:/bin", "HOME=/tmp", "LANG=C", "TMPDIR=/tmp"} {
		if !env[v] {
			return ErrPolicy
		}
	}
	expected := map[string]string{"/workspace": source, "/context": bundle, "/ep-guard": guard}
	if len(c.Mounts) != 3 {
		return ErrPolicy
	}
	for _, m := range c.Mounts {
		src, ok := expected[m.Destination]
		if !ok || src != m.Source || m.Type != "bind" || m.RW || m.Propagation != "rprivate" {
			return ErrPolicy
		}
		delete(expected, m.Destination)
	}
	if len(h.Tmpfs) != 1 || h.Tmpfs["/tmp"] != "rw,nosuid,nodev,noexec,size=67108864,mode=1777" {
		return ErrPolicy
	}
	return nil
}

type containerLogConfig struct {
	Type   string
	Config map[string]string
}
