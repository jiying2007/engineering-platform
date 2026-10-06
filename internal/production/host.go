package production

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jiying2007/engineering-platform/internal/access"
	"github.com/jiying2007/engineering-platform/internal/controlclient"
	"github.com/jiying2007/engineering-platform/internal/distribution"
	"github.com/jiying2007/engineering-platform/internal/githubpublish"
	"github.com/jiying2007/engineering-platform/internal/preparation"
	"github.com/jiying2007/engineering-platform/internal/strictjson"
)

// Check verifies local host facts only. No database connection, provider turn,
// publisher mutation or service restart is attempted. HOST_VALIDATED cannot be
// substituted for the separately authenticated operational/live acceptance.
func Check(c Config) (Result, error) {
	result, err := CheckConfiguration(c)
	if err != nil {
		return result, err
	}
	users := []string{c.ControlServiceUser, c.AdmissionServiceUser, c.PreparationServiceUser, c.PublisherServiceUser}
	uids, err := resolveUsers(users, user.Lookup)
	if err != nil {
		return result, err
	}
	groups, err := resolveServiceGroups(users, user.Lookup)
	if err != nil {
		return result, err
	}
	dir := filepath.Dir(c.ControlBinary)
	for role, path := range map[string]string{"control-plane": c.ControlBinary, "worker": c.WorkerBinary, "eng": c.EngBinary, "publisher-service": c.PublisherBinary} {
		if path != filepath.Join(dir, role) {
			return result, fmt.Errorf("role paths must select one complete distribution")
		}
	}
	dist, err := distribution.Verify(dir)
	if err != nil {
		return result, fmt.Errorf("distribution: %w", err)
	}
	if dist.SourceCommit != c.ExpectedSourceCommit {
		return result, fmt.Errorf("distribution is not the operator-admitted source commit")
	}
	for _, name := range distribution.Names() {
		info, e := os.Stat(filepath.Join(dir, name))
		if e != nil {
			return result, e
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return result, fmt.Errorf("Unix executable ownership required")
		}
		for _, uid := range uids {
			if int(stat.Uid) == uid {
				return result, fmt.Errorf("runtime identity must not own delivered executables")
			}
		}
	}
	envs := make([]map[string]string, 0, 4)
	for i, path := range []string{c.ControlEnvFile, c.AdmissionEnvFile, c.PreparationEnvFile, c.PublisherEnvFile} {
		if err := hostFile(path, true, uids[i], true); err != nil {
			return result, err
		}
		values, e := readEnvFile(path)
		if e != nil {
			return result, e
		}
		for key, value := range values {
			if strings.HasSuffix(key, "_FILE") || strings.HasSuffix(key, "_CONFIG") {
				if e := hostFile(value, strings.HasSuffix(key, "_KEY_FILE"), uids[i], false); e != nil {
					return result, fmt.Errorf("%s: %w", key, e)
				}
				if e := servicePathAccess(value, uids[i], groups[i], 4); e != nil {
					return result, fmt.Errorf("%s: %w", key, e)
				}
			}
		}
		envs = append(envs, values)
	}
	control, admission, prepare, publisher := envs[0], envs[1], envs[2], envs[3]
	if err := checkServerTLS(control, "CONTROL"); err != nil {
		return result, err
	}
	policy, err := access.ReadConfiguration(control["CONTROL_AUTH_POLICY_FILE"], false)
	if err != nil {
		return result, err
	}
	if _, err = access.Decode(policy); err != nil {
		return result, fmt.Errorf("invalid control access policy")
	}
	for _, values := range []map[string]string{admission, prepare} {
		client, e := controlclient.FromEnvironment(func(k string) string { return values[k] })
		if e != nil {
			return result, e
		}
		client.Close()
	}
	if err := checkServerTLS(publisher, "PUBLISHER"); err != nil {
		return result, err
	}
	var remote githubpublish.RemoteConfiguration
	if err := readHostJSON(control["GITHUB_PUBLISHER_REMOTE_FILE"], &remote); err != nil {
		return result, err
	}
	for _, path := range []string{remote.ClientCertFile, remote.ClientKeyFile, remote.ServerCAFile} {
		if err := hostFile(path, path == remote.ClientKeyFile, uids[0], false); err != nil {
			return result, err
		}
		if err := servicePathAccess(path, uids[0], groups[0], 4); err != nil {
			return result, err
		}
	}
	if _, err := githubpublish.NewRemoteClient(remote); err != nil {
		return result, err
	}
	wantEndpoint := "https://" + net.JoinHostPort(publisher["LISTEN_HOST"], publisher["PORT"])
	if strings.TrimSuffix(remote.Endpoint, "/") != wantEndpoint {
		return result, fmt.Errorf("publisher endpoint does not match deployed listener")
	}
	var plan githubpublish.PlanConfiguration
	if err := readHostJSON(control["GITHUB_PUBLISHER_PLAN_FILE"], &plan); err != nil {
		return result, err
	}
	var pub githubpublish.Configuration
	if err := readHostJSON(publisher["PUBLISHER_CONFIG_FILE"], &pub); err != nil {
		return result, err
	}
	if plan.Version != 1 || pub.Version != 1 || len(plan.Targets) == 0 || len(plan.Targets) != len(pub.Targets) || plan.ArtifactRoot != pub.ArtifactRoot {
		return result, fmt.Errorf("publisher plan/service policy mismatch")
	}
	for i, target := range plan.Targets {
		if target != pub.Targets[i] {
			return result, fmt.Errorf("publisher target policy mismatch")
		}
	}
	if err := safeDirectory(pub.ArtifactRoot); err != nil {
		return result, err
	}
	if err := servicePathAccess(pub.ArtifactRoot, uids[3], groups[3], 5); err != nil {
		return result, fmt.Errorf("publisher artifact root: %w", err)
	}
	if err := safeExecutable(pub.GitExecutable); err != nil {
		return result, err
	}
	if err := hostFile(pub.TokenFile, true, uids[3], false); err != nil {
		return result, err
	}
	if err := servicePathAccess(pub.TokenFile, uids[3], groups[3], 4); err != nil {
		return result, err
	}
	// This constructor validates policy/configuration and reads the token locally;
	// it performs no GitHub request and proves no account/token qualification.
	if _, err := githubpublish.LoadRemoteService(publisher["PUBLISHER_CONFIG_FILE"], publisher["PUBLISHER_CONTROL_SUBJECT"]); err != nil {
		return result, err
	}
	var pc preparation.Configuration
	if err := readHostJSON(prepare["WORKER_PREPARATION_CONFIG"], &pc); err != nil {
		return result, err
	}
	if pc.Version != 1 || !strings.HasPrefix(pc.Worker, "urn:engineering-platform:") || len(pc.Approvals) == 0 {
		return result, fmt.Errorf("invalid preparation configuration")
	}
	for index, path := range []string{pc.Root, pc.ContextSource} {
		if err := safeDirectory(path); err != nil {
			return result, err
		}
		want := uint32(5)
		if index == 0 {
			want = 7
		}
		if err := servicePathAccess(path, uids[2], groups[2], want); err != nil {
			return result, err
		}
	}
	if err := safeExecutable(pc.Git); err != nil {
		return result, err
	}
	worker, e := controlclient.FromEnvironment(func(k string) string { return prepare[k] })
	if e != nil {
		return result, e
	}
	defer worker.Close()
	if worker.Subject() != pc.Worker {
		return result, fmt.Errorf("preparation policy does not belong to its TLS identity")
	}
	result.ControlPlane = StateHostValidated
	result.WorkerAdmission = StateHostValidated
	result.WorkerPreparation = StateHostValidated
	result.Publisher = StateHostValidated
	result.Internal = StateHostValidated
	result.HostValidated = true
	result.SourceCommit = dist.SourceCommit
	result.Blockers = []string{"unattended_provider_live_qualification", "live_service_operational_qualification"}
	return result, nil
}

func resolveServiceGroups(names []string, lookup func(string) (*user.User, error)) ([]map[uint32]bool, error) {
	out := make([]map[uint32]bool, 0, len(names))
	for _, name := range names {
		account, err := lookup(name)
		if err != nil || account == nil {
			return nil, fmt.Errorf("service user %s does not exist", name)
		}
		ids, err := account.GroupIds()
		if err != nil {
			return nil, fmt.Errorf("service user %s groups unavailable", name)
		}
		ids = append(ids, account.Gid)
		set := map[uint32]bool{}
		for _, raw := range ids {
			value, err := strconv.ParseUint(raw, 10, 32)
			if err != nil || value == 0 {
				return nil, fmt.Errorf("service user %s has invalid group identity", name)
			}
			set[uint32(value)] = true
		}
		out = append(out, set)
	}
	return out, nil
}

func resolveUsers(names []string, lookup func(string) (*user.User, error)) ([]int, error) {
	out := make([]int, 0, len(names))
	seen := map[int]bool{}
	for _, name := range names {
		account, err := lookup(name)
		if err != nil || account == nil {
			return nil, fmt.Errorf("service user %s does not exist", name)
		}
		uid, err := strconv.Atoi(account.Uid)
		if err != nil || uid <= 0 || seen[uid] {
			return nil, fmt.Errorf("distinct non-root service UIDs required")
		}
		seen[uid] = true
		out = append(out, uid)
	}
	return out, nil
}
func validateListen(values map[string]string) error {
	if net.ParseIP(values["LISTEN_HOST"]) == nil {
		return fmt.Errorf("literal listener IP required")
	}
	port, err := strconv.Atoi(values["PORT"])
	if err != nil || port < 1 || port > 65535 || strconv.Itoa(port) != values["PORT"] {
		return fmt.Errorf("canonical TCP port 1..65535 required")
	}
	return nil
}
func validateEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("root HTTPS endpoint required")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid endpoint port")
		}
	}
	return nil
}
func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || strings.Trim(u.Path, "/") == "" || u.Fragment != "" {
		return fmt.Errorf("explicit PostgreSQL URL with user, host and database required")
	}
	if _, err := pgx.ParseConfig(raw); err != nil {
		return fmt.Errorf("invalid PostgreSQL configuration")
	}
	return nil
}
func hostFile(path string, secret bool, uid int, systemdEnv bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("absolute normalized configuration path required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("configuration missing or aliased")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() <= 0 || info.Size() > 1<<20 || (secret && info.Mode().Perm()&0o077 != 0) {
		return fmt.Errorf("unsafe referenced configuration file")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("Unix configuration ownership required")
	}
	if stat.Nlink != 1 {
		return fmt.Errorf("configuration hard-link alias is not allowed")
	}
	if secret && int(stat.Uid) != uid && !(systemdEnv && stat.Uid == 0) {
		return fmt.Errorf("private configuration is not readable by its service identity")
	}
	return nil
}

func unixModeAllows(info os.FileInfo, uid int, gids map[uint32]bool, want uint32) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	shift := uint(0)
	if int(stat.Uid) == uid {
		shift = 6
	} else if gids[stat.Gid] {
		shift = 3
	}
	actual := (uint32(info.Mode().Perm()) >> shift) & 7
	return actual&want == want
}

func servicePathAccess(path string, uid int, gids map[uint32]bool, want uint32) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || uid <= 0 || len(gids) == 0 || want == 0 || want&^uint32(7) != 0 {
		return fmt.Errorf("invalid service path-access contract")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return fmt.Errorf("service path missing or aliased")
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil || !info.IsDir() || !unixModeAllows(info, uid, gids, 1) {
			return fmt.Errorf("service identity cannot traverse referenced path")
		}
		if parent == string(filepath.Separator) {
			break
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !unixModeAllows(info, uid, gids, want) {
		return fmt.Errorf("service identity lacks required mode-bit access")
	}
	return nil
}

func readHostJSON(path string, out any) error {
	data, err := access.ReadConfiguration(path, false)
	if err != nil {
		return err
	}
	if err := strictjson.Decode(data, out); err != nil {
		return fmt.Errorf("invalid referenced configuration")
	}
	return nil
}
func checkServerTLS(v map[string]string, prefix string) error {
	config, err := access.LoadServerTLS(v[prefix+"_TLS_CERT_FILE"], v[prefix+"_TLS_KEY_FILE"], v[prefix+"_CLIENT_CA_FILE"])
	if err != nil {
		return err
	}
	pair := config.Certificates[0]
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	now := time.Now()
	if err != nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return fmt.Errorf("current server certificate required")
	}
	if err := leaf.VerifyHostname(v["LISTEN_HOST"]); err != nil {
		return fmt.Errorf("server certificate does not match listener")
	}
	usage := false
	for _, u := range leaf.ExtKeyUsage {
		usage = usage || u == x509.ExtKeyUsageServerAuth
	}
	if !usage {
		return fmt.Errorf("explicit serverAuth usage required")
	}
	if config.ClientAuth != tls.RequireAndVerifyClientCert {
		return fmt.Errorf("mutual TLS required")
	}
	return nil
}
