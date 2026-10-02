// Command whatsapp-doppel is a local web app that lets LLM personas reply in
// selected WhatsApp chats.
//
//	whatsapp-doppel                 launch (what the .app runs): open the running
//	                                instance in the browser, or start one in the background
//	whatsapp-doppel serve [flags]   run the server in the foreground
//	whatsapp-doppel status          show whether a server is running
//	whatsapp-doppel quit            stop the running server
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"whatsappdoppel/internal/app"
	"whatsappdoppel/internal/config"
	"whatsappdoppel/internal/launcher"
)

// Set via -ldflags "-X main.version=... -X main.devProjectDir=...".
var (
	version       = "dev"
	devProjectDir = ""
)

func main() {
	args := os.Args[1:]
	// Older macOS versions pass -psn_0_NNN to apps started from Finder.
	if len(args) > 0 && strings.HasPrefix(args[0], "-psn") {
		args = args[1:]
	}
	cmd := "launch"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "launch":
		err = runLaunch(args)
	case "serve":
		err = runServe(args)
	case "status":
		err = runStatus(args)
	case "quit", "stop":
		err = runQuit(args)
	case "version", "--version":
		fmt.Println("whatsapp-doppel", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		if !errors.Is(err, app.ErrAlreadyRunning) && !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}

// errSilent exits with status 1 without printing anything more.
var errSilent = errors.New("silent failure")

func usage() {
	fmt.Fprint(os.Stderr, `WhatsApp Doppel `+version+`

Usage:
  whatsapp-doppel [launch] [--data-dir D] [--fake-wa] [--fake-llm]
                                            open the app (starts the server in the background if needed)
  whatsapp-doppel serve [flags]             run the server in the foreground
      --port N          port to listen on (default: settings, then 7788, 7789, 8080, ...)
      --data-dir D      data folder (default: ~/Library/Application Support/WhatsappDoppel or $DOPPEL_DATA_DIR)
      --legacy-db F     import this old bot.db WhatsApp session on first start
      --fake-wa         use a simulated WhatsApp (no phone needed)
      --fake-llm        use a canned LLM (no Ollama/API key needed)
      --open            open the browser once the server is up
  whatsapp-doppel status [--data-dir D]     show the running server
  whatsapp-doppel quit [--data-dir D]       stop the running server
  whatsapp-doppel version

Environment: DOPPEL_DATA_DIR, DOPPEL_PORT, DOPPEL_LEGACY_DB, DOPPEL_NO_BROWSER=1
`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = usage
	return fs
}

func runLaunch(args []string) error {
	fs := newFlagSet("launch")
	dataDir := fs.String("data-dir", "", "data folder")
	fakeWA := fs.Bool("fake-wa", false, "start the server with a simulated WhatsApp")
	fakeLLM := fs.Bool("fake-llm", false, "start the server with a canned LLM")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var extra []string
	if *fakeWA {
		extra = append(extra, "--fake-wa")
	}
	if *fakeLLM {
		extra = append(extra, "--fake-llm")
	}
	interactive := isTerminal(os.Stderr)
	logf := func(format string, a ...any) {
		if interactive {
			fmt.Fprintf(os.Stderr, format+"\n", a...)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	opts := launcher.Options{DataDir: *dataDir, ExtraArgs: extra, Logf: logf}
	if noBrowser() {
		opts.Open = func(string) error { return nil }
	}
	url, err := launcher.Launch(ctx, opts)
	if err != nil {
		if !interactive {
			// Started from Finder: there is no terminal to print to.
			launcher.ShowAlert("WhatsApp Doppel could not start", err.Error())
		}
		return err
	}
	logf("WhatsApp Doppel is running at %s", url)
	return nil
}

func runServe(args []string) error {
	fs := newFlagSet("serve")
	var o app.Options
	fs.IntVar(&o.Port, "port", 0, "port")
	fs.StringVar(&o.DataDir, "data-dir", "", "data folder")
	fs.StringVar(&o.LegacyDB, "legacy-db", "", "legacy bot.db to import")
	fs.BoolVar(&o.FakeWA, "fake-wa", false, "simulated WhatsApp")
	fs.BoolVar(&o.FakeLLM, "fake-llm", false, "canned LLM")
	fs.BoolVar(&o.OpenBrowser, "open", false, "open the browser")
	fs.BoolVar(&o.FromLauncher, "from-launcher", false, "started by the launcher (internal)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.Version = version
	o.DevProjectDir = devProjectDir
	if noBrowser() {
		o.OpenBrowser = false
	}
	return app.Run(o)
}

func dataPaths(args []string, name string) (config.Paths, error) {
	fs := newFlagSet(name)
	dataDir := fs.String("data-dir", "", "data folder")
	if err := fs.Parse(args); err != nil {
		return config.Paths{}, err
	}
	return config.NewPaths(*dataDir)
}

func runStatus(args []string) error {
	paths, err := dataPaths(args, "status")
	if err != nil {
		return err
	}
	inst, ok := launcher.Running(context.Background(), paths)
	if !ok {
		fmt.Println("WhatsApp Doppel is not running (data:", paths.Dir+")")
		return errSilent
	}
	fmt.Printf("WhatsApp Doppel %s is running at %s\n  pid %d, started %s\n  data: %s\n",
		inst.Version, inst.URL(), inst.PID, inst.StartedAt.Local().Format(time.DateTime), paths.Dir)
	return nil
}

func runQuit(args []string) error {
	paths, err := dataPaths(args, "quit")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := launcher.Quit(ctx, paths, 15*time.Second); err != nil {
		if launcher.IsNotRunning(err) {
			fmt.Println("WhatsApp Doppel is not running")
			return nil
		}
		return err
	}
	fmt.Println("WhatsApp Doppel stopped")
	return nil
}

// noBrowser lets scripts and tests suppress opening a browser (DOPPEL_NO_BROWSER=1).
func noBrowser() bool { return os.Getenv("DOPPEL_NO_BROWSER") != "" }

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TIOCGETA)
	return err == nil
}
