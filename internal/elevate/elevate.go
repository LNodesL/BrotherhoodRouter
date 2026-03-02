package elevate

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

const MarkerArg = "--_bhrouter_elevated"

// MaybeRelaunchForWrite requests elevation if the hosts file cannot be written.
// It returns restarted=true if a child elevated process was launched.
func MaybeRelaunchForWrite(writePath string, alreadyElevated bool) (bool, error) {
	if err := canWriteFile(writePath); err == nil {
		return false, nil
	} else if !isPermissionDenied(err) {
		return false, err
	}

	if alreadyElevated {
		return false, fmt.Errorf("insufficient privileges to write %s even after elevation attempt", writePath)
	}

	exe, err := os.Executable()
	if err != nil {
		return false, fmt.Errorf("resolve executable path: %w", err)
	}

	args := filterMarkerArg(os.Args[1:])
	args = append(args, MarkerArg)

	switch runtime.GOOS {
	case "darwin", "linux":
		cmdArgs := append([]string{"-E", exe}, args...)
		cmd := exec.Command("sudo", cmdArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return true, fmt.Errorf("elevated run failed: %w", err)
		}
		return true, nil
	case "windows":
		if err := runWindowsElevated(exe, args); err != nil {
			return true, fmt.Errorf("elevated run failed: %w", err)
		}
		return true, nil
	default:
		return false, fmt.Errorf("automatic elevation is not supported on %s; rerun with admin privileges", runtime.GOOS)
	}
}

func canWriteFile(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	return f.Close()
}

func isPermissionDenied(err error) bool {
	if os.IsPermission(err) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "permission denied") || strings.Contains(lower, "access is denied")
}

func filterMarkerArg(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == MarkerArg {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func runWindowsElevated(exe string, args []string) error {
	argListParts := make([]string, 0, len(args))
	for _, a := range args {
		argListParts = append(argListParts, psSingleQuote(a))
	}
	argList := "@()"
	if len(argListParts) > 0 {
		argList = "@(" + strings.Join(argListParts, ",") + ")"
	}
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		fmt.Sprintf("$p=Start-Process -FilePath %s -ArgumentList %s -Verb RunAs -Wait -PassThru; exit $p.ExitCode", psSingleQuote(exe), argList),
	)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
