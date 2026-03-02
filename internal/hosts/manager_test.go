package hosts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetCreatesManagedBlock(t *testing.T) {
	t.Parallel()

	path := writeTempHosts(t, "# comment\n127.0.0.1 localhost\n")
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.Set("example.com", "127.0.0.55"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got := readFile(t, path)
	if !strings.Contains(got, managedStart) || !strings.Contains(got, managedEnd) {
		t.Fatalf("missing managed markers:\n%s", got)
	}
	if !strings.Contains(got, "127.0.0.55\texample.com\t# bhrouter") {
		t.Fatalf("missing mapping line:\n%s", got)
	}

	snap, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snap.Managed) != 1 || snap.Managed[0].Host != "example.com" || snap.Managed[0].IP != "127.0.0.55" {
		t.Fatalf("unexpected snapshot: %+v", snap.Managed)
	}
}

func TestRemoveLastEntryRemovesBlock(t *testing.T) {
	t.Parallel()

	path := writeTempHosts(t, "# comment\n127.0.0.1 localhost\n")
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.Set("example.com", "127.0.0.1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	removed, err := m.Remove("example.com")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Fatalf("expected Remove to return removed=true")
	}

	got := readFile(t, path)
	if strings.Contains(got, managedStart) || strings.Contains(got, managedEnd) {
		t.Fatalf("expected block markers removed:\n%s", got)
	}
}

func TestListConflicts(t *testing.T) {
	t.Parallel()

	path := writeTempHosts(t, "# comment\n10.1.2.3 example.com\n127.0.0.1 localhost\n")
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.Set("example.com", "127.0.0.88"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	snap, err := m.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(snap.Conflicts) != 1 {
		t.Fatalf("expected one conflict, got %d", len(snap.Conflicts))
	}
	if snap.Conflicts[0].Host != "example.com" || snap.Conflicts[0].IP != "10.1.2.3" {
		t.Fatalf("unexpected conflict: %+v", snap.Conflicts[0])
	}
}

func TestSetRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	path := writeTempHosts(t, "127.0.0.1 localhost\n")
	m, err := NewManager(path)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if err := m.Set("bad host", "127.0.0.1"); err == nil {
		t.Fatalf("expected invalid host error")
	}
	if err := m.Set("example.com", "not-an-ip"); err == nil {
		t.Fatalf("expected invalid ip error")
	}
}

func writeTempHosts(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp hosts: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}
