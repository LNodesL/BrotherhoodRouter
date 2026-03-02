package hosts

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	managedStart = "# >>> BHRouter managed block >>>"
	managedEnd   = "# <<< BHRouter managed block <<<"
	managedNote  = "# managed by BHRouter v0.0.1"
)

// Entry is one host override rule.
type Entry struct {
	Host string `json:"host"`
	IP   string `json:"ip"`
}

// Snapshot contains the managed entries and potential shadowing conflicts.
type Snapshot struct {
	Managed   []Entry `json:"managed"`
	Conflicts []Entry `json:"conflicts"`
	Path      string  `json:"path"`
}

// Manager manipulates one hosts file.
type Manager struct {
	Path string
}

type fileState struct {
	Newline       string
	BaseLines     []string
	Managed       map[string]string
	UnmanagedSeen map[string]string
	InsertAt      int
}

// NewManager creates a manager for a specific hosts file path.
func NewManager(path string) (*Manager, error) {
	if strings.TrimSpace(path) == "" {
		p, err := DefaultHostsPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &Manager{Path: path}, nil
}

// DefaultHostsPath returns the canonical hosts path for this OS.
func DefaultHostsPath() (string, error) {
	switch runtime.GOOS {
	case "windows":
		systemRoot := os.Getenv("SystemRoot")
		if systemRoot == "" {
			systemRoot = `C:\\Windows`
		}
		return filepath.Join(systemRoot, "System32", "drivers", "etc", "hosts"), nil
	case "darwin", "linux", "freebsd", "openbsd", "netbsd":
		return "/etc/hosts", nil
	default:
		return "", fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
}

// List returns BHRouter-managed entries and conflicts with unmanaged entries.
func (m *Manager) List() (*Snapshot, error) {
	state, err := m.readState()
	if err != nil {
		return nil, err
	}

	managed := make([]Entry, 0, len(state.Managed))
	for host, ip := range state.Managed {
		managed = append(managed, Entry{Host: host, IP: ip})
	}
	sortEntries(managed)

	conflicts := make([]Entry, 0)
	for _, e := range managed {
		if unmanagedIP, ok := state.UnmanagedSeen[e.Host]; ok && unmanagedIP != e.IP {
			conflicts = append(conflicts, Entry{Host: e.Host, IP: unmanagedIP})
		}
	}

	return &Snapshot{Managed: managed, Conflicts: conflicts, Path: m.Path}, nil
}

// Set inserts or updates one managed override.
func (m *Manager) Set(host, ip string) error {
	host, ip, err := validateInput(host, ip)
	if err != nil {
		return err
	}

	state, original, err := m.readStateWithContent()
	if err != nil {
		return err
	}

	if current, ok := state.Managed[host]; ok && current == ip {
		return nil
	}
	state.Managed[host] = ip

	rendered := render(state)
	if rendered == original {
		return nil
	}
	return m.writeWithBackup(rendered)
}

// Remove deletes one managed override.
func (m *Manager) Remove(host string) (bool, error) {
	host = normalizeHost(host)
	if err := validateHost(host); err != nil {
		return false, err
	}

	state, original, err := m.readStateWithContent()
	if err != nil {
		return false, err
	}
	if _, ok := state.Managed[host]; !ok {
		return false, nil
	}
	delete(state.Managed, host)

	rendered := render(state)
	if rendered == original {
		return true, nil
	}
	if err := m.writeWithBackup(rendered); err != nil {
		return false, err
	}
	return true, nil
}

// Backup writes a timestamped copy of the hosts file next to the original.
func (m *Manager) Backup() (string, error) {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return "", fmt.Errorf("read hosts file: %w", err)
	}
	stamp := time.Now().Format("20060102-150405")
	backupPath := fmt.Sprintf("%s.bhrouter.backup-%s", m.Path, stamp)
	if err := os.WriteFile(backupPath, content, 0o600); err != nil {
		return "", fmt.Errorf("write backup %q: %w", backupPath, err)
	}
	return backupPath, nil
}

func (m *Manager) readStateWithContent() (*fileState, string, error) {
	content, err := os.ReadFile(m.Path)
	if err != nil {
		return nil, "", fmt.Errorf("read hosts file %q: %w", m.Path, err)
	}
	state, renderedOriginal, err := parse(string(content))
	if err != nil {
		return nil, "", fmt.Errorf("parse hosts file %q: %w", m.Path, err)
	}
	return state, renderedOriginal, nil
}

func (m *Manager) readState() (*fileState, error) {
	state, _, err := m.readStateWithContent()
	return state, err
}

func (m *Manager) writeWithBackup(content string) error {
	info, err := os.Stat(m.Path)
	if err != nil {
		return fmt.Errorf("stat hosts file %q: %w", m.Path, err)
	}

	if _, err := m.Backup(); err != nil {
		return err
	}

	dir := filepath.Dir(m.Path)
	tmpFile, err := os.CreateTemp(dir, ".bhrouter-hosts-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write temp hosts file: %w", err)
	}
	if err := tmpFile.Chmod(info.Mode().Perm()); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod temp hosts file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("sync temp hosts file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp hosts file: %w", err)
	}

	if runtime.GOOS == "windows" {
		_ = os.Remove(m.Path)
	}
	if err := os.Rename(tmpName, m.Path); err != nil {
		return fmt.Errorf("replace hosts file: %w", err)
	}
	return nil
}

func parse(content string) (*fileState, string, error) {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	managed := make(map[string]string)
	unmanagedSeen := make(map[string]string)
	baseLines := make([]string, 0, len(lines))

	inManaged := false
	foundStart := false
	foundEnd := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == managedStart:
			if foundStart && !foundEnd {
				return nil, "", errors.New("multiple managed block starts")
			}
			inManaged = true
			foundStart = true
			continue
		case trimmed == managedEnd:
			if !inManaged {
				return nil, "", errors.New("managed block end without start")
			}
			inManaged = false
			foundEnd = true
			continue
		}

		if inManaged {
			ip, hosts, ok := parseMappingLine(line)
			if ok {
				for _, h := range hosts {
					h := normalizeHost(h)
					if validateHost(h) == nil {
						managed[h] = ip
					}
				}
			}
			continue
		}

		baseLines = append(baseLines, line)
		ip, hosts, ok := parseMappingLine(line)
		if ok {
			for _, h := range hosts {
				h := normalizeHost(h)
				if validateHost(h) == nil {
					if _, exists := unmanagedSeen[h]; !exists {
						unmanagedSeen[h] = ip
					}
				}
			}
		}
	}

	if inManaged {
		return nil, "", errors.New("managed block not terminated")
	}
	if foundStart != foundEnd {
		return nil, "", errors.New("managed block markers are unbalanced")
	}

	insertAt := findInsertionIndex(baseLines)
	state := &fileState{
		Newline:       newline,
		BaseLines:     baseLines,
		Managed:       managed,
		UnmanagedSeen: unmanagedSeen,
		InsertAt:      insertAt,
	}
	return state, render(state), nil
}

func render(state *fileState) string {
	out := make([]string, 0, len(state.BaseLines)+len(state.Managed)+6)
	before := state.BaseLines[:state.InsertAt]
	after := state.BaseLines[state.InsertAt:]

	out = append(out, before...)

	if len(state.Managed) > 0 {
		if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}

		out = append(out, managedStart)
		out = append(out, managedNote)

		entries := make([]Entry, 0, len(state.Managed))
		for host, ip := range state.Managed {
			entries = append(entries, Entry{Host: host, IP: ip})
		}
		sortEntries(entries)
		for _, e := range entries {
			out = append(out, fmt.Sprintf("%s\t%s\t# bhrouter", e.IP, e.Host))
		}
		out = append(out, managedEnd)

		if len(after) > 0 && strings.TrimSpace(after[0]) != "" {
			out = append(out, "")
		}
	}

	out = append(out, after...)
	return strings.Join(out, state.Newline) + state.Newline
}

func findInsertionIndex(lines []string) int {
	idx := 0
	for idx < len(lines) {
		trimmed := strings.TrimSpace(lines[idx])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			idx++
			continue
		}
		break
	}
	return idx
}

func parseMappingLine(line string) (string, []string, bool) {
	candidate := line
	if hash := strings.Index(candidate, "#"); hash >= 0 {
		candidate = candidate[:hash]
	}
	fields := strings.Fields(candidate)
	if len(fields) < 2 {
		return "", nil, false
	}
	ip := fields[0]
	if net.ParseIP(ip) == nil {
		return "", nil, false
	}
	return ip, fields[1:], true
}

func validateInput(host, ip string) (string, string, error) {
	host = normalizeHost(host)
	if err := validateHost(host); err != nil {
		return "", "", err
	}

	parsedIP := net.ParseIP(strings.TrimSpace(ip))
	if parsedIP == nil {
		return "", "", fmt.Errorf("invalid IP address %q", ip)
	}
	return host, parsedIP.String(), nil
}

func normalizeHost(host string) string {
	host = strings.TrimSpace(host)
	host = strings.TrimSuffix(host, ".")
	return strings.ToLower(host)
}

func validateHost(host string) error {
	if host == "" {
		return errors.New("host cannot be empty")
	}
	if len(host) > 253 {
		return fmt.Errorf("host %q exceeds 253 characters", host)
	}
	labels := strings.Split(host, ".")
	for _, label := range labels {
		if label == "" {
			return fmt.Errorf("host %q contains empty label", host)
		}
		if len(label) > 63 {
			return fmt.Errorf("label %q in host %q exceeds 63 characters", label, host)
		}
		for i, r := range label {
			valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
			if !valid {
				return fmt.Errorf("host %q contains invalid character %q", host, r)
			}
			if (i == 0 || i == len(label)-1) && r == '-' {
				return fmt.Errorf("label %q in host %q cannot start or end with '-'", label, host)
			}
		}
	}
	return nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Host == entries[j].Host {
			return entries[i].IP < entries[j].IP
		}
		return entries[i].Host < entries[j].Host
	})
}
