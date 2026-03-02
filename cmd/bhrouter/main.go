package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"

	"bhrouter/internal/elevate"
	"bhrouter/internal/hosts"
	"bhrouter/internal/ui"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cleanArgs, opts, err := extractGlobalOptions(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		printUsage()
		return 2
	}
	if len(cleanArgs) == 0 {
		printUsage()
		return 1
	}

	command := cleanArgs[0]
	commandArgs := cleanArgs[1:]
	if command == "--ui" {
		command = "ui"
	}

	manager, err := hosts.NewManager(opts.hostsPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}

	switch command {
	case "help", "-h", "--help":
		printUsage()
		return 0
	case "path":
		fmt.Println(manager.Path)
		return 0
	case "list":
		return runList(manager)
	case "set":
		if len(commandArgs) != 2 {
			fmt.Fprintln(os.Stderr, "Usage: bhrouter set <host> <ip>")
			return 2
		}
		if code := ensureElevatedIfNeeded(manager.Path, opts.alreadyElevated); code == -1 {
			return 0
		} else if code != 0 {
			return code
		}
		if err := manager.Set(commandArgs[0], commandArgs[1]); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		fmt.Printf("Set %s -> %s\n", commandArgs[0], commandArgs[1])
		return 0
	case "remove", "rm", "delete":
		if len(commandArgs) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: bhrouter remove <host>")
			return 2
		}
		if code := ensureElevatedIfNeeded(manager.Path, opts.alreadyElevated); code == -1 {
			return 0
		} else if code != 0 {
			return code
		}
		removed, err := manager.Remove(commandArgs[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		if removed {
			fmt.Printf("Removed %s\n", commandArgs[0])
		} else {
			fmt.Printf("No managed override found for %s\n", commandArgs[0])
		}
		return 0
	case "backup":
		if code := ensureElevatedIfNeeded(manager.Path, opts.alreadyElevated); code == -1 {
			return 0
		} else if code != 0 {
			return code
		}
		path, err := manager.Backup()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		fmt.Println(path)
		return 0
	case "ui":
		return runUI(manager, commandArgs, opts.alreadyElevated)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command %q\n", command)
		printUsage()
		return 2
	}
}

type globalOptions struct {
	hostsPath       string
	alreadyElevated bool
}

func extractGlobalOptions(args []string) ([]string, globalOptions, error) {
	out := make([]string, 0, len(args))
	var opts globalOptions

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == elevate.MarkerArg:
			opts.alreadyElevated = true
		case a == "--hosts":
			if i+1 >= len(args) {
				return nil, opts, errors.New("--hosts requires a value")
			}
			opts.hostsPath = args[i+1]
			i++
		case strings.HasPrefix(a, "--hosts="):
			opts.hostsPath = strings.TrimPrefix(a, "--hosts=")
		default:
			out = append(out, a)
		}
	}
	return out, opts, nil
}

func ensureElevatedIfNeeded(hostsPath string, alreadyElevated bool) int {
	restarted, err := elevate.MaybeRelaunchForWrite(hostsPath, alreadyElevated)
	if restarted {
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			return 1
		}
		return -1
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func runList(manager *hosts.Manager) int {
	snapshot, err := manager.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}

	fmt.Printf("Hosts file: %s\n", snapshot.Path)
	if len(snapshot.Managed) == 0 {
		fmt.Println("No managed overrides.")
	} else {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "HOST\tIP")
		for _, e := range snapshot.Managed {
			fmt.Fprintf(w, "%s\t%s\n", e.Host, e.IP)
		}
		_ = w.Flush()
	}

	if len(snapshot.Conflicts) > 0 {
		fmt.Println("\nPotential conflicts (existing unmanaged entries):")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "HOST\tEXISTING_IP")
		for _, e := range snapshot.Conflicts {
			fmt.Fprintf(w, "%s\t%s\n", e.Host, e.IP)
		}
		_ = w.Flush()
	}
	return 0
}

func runUI(manager *hosts.Manager, args []string, alreadyElevated bool) int {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 8787, "port for the local UI")
	noOpen := fs.Bool("no-open", false, "do not auto-open browser")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if code := ensureElevatedIfNeeded(manager.Path, alreadyElevated); code == -1 {
		return 0
	} else if code != 0 {
		return code
	}

	h := ui.NewServer(manager)
	addr := "127.0.0.1:" + strconv.Itoa(*port)
	url := "http://" + addr

	if !*noOpen {
		go openBrowser(url)
	}

	fmt.Printf("BHRouter UI running at %s\n", url)
	fmt.Println("Press Ctrl+C to stop")

	srv := &http.Server{
		Addr:    addr,
		Handler: h.Handler(),
	}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "Error:", err)
		return 1
	}
	return 0
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func printUsage() {
	fmt.Println(`BHRouter (Brotherhood Router v0.0.1)

Usage:
  bhrouter [--hosts <path>] <command> [args]

Commands:
  list                  List BHRouter-managed overrides
  set <host> <ip>       Add/update one override
  remove <host>         Remove one managed override
  backup                Create a backup of the hosts file
  path                  Print hosts file path for current OS
  ui [--port 8787]      Launch local browser UI
  help                  Show this help

Examples:
  bhrouter list
  bhrouter set example.com 127.0.0.1
  bhrouter remove example.com
  bhrouter ui --port 8787

Cross-compile examples:
  GOOS=darwin GOARCH=arm64 go build -o dist/bhrouter-darwin-arm64 ./cmd/bhrouter
  GOOS=linux  GOARCH=amd64 go build -o dist/bhrouter-linux-amd64  ./cmd/bhrouter
  GOOS=windows GOARCH=amd64 go build -o dist/bhrouter-windows-amd64.exe ./cmd/bhrouter`)
}
