package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/0xr3ngar/veil/internal/blocker"
	"github.com/0xr3ngar/veil/internal/config"
	vdns "github.com/0xr3ngar/veil/internal/dns"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "start":
		cmdStart()
	case "stop":
		cmdStop()
	case "status":
		cmdStatus()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: veil <command>

Commands:
  start     Start the DNS proxy and web UI
  stop      Stop the running daemon
  status    Show running status`)
}

func cmdStart() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	if err := cfg.Save(); err != nil {
		log.Printf("warning: could not save default config: %v", err)
	}

	b := blocker.New()
	log.Println("loading blocklists...")
	b.Reload(cfg)
	log.Printf("loaded %d blocked domains", len(b.BlockedDomains()))

	srv := vdns.NewServer(b, cfg.DNSListen, cfg.UpstreamDNS, cfg.RedirectTo)

	if err := writePID(); err != nil {
		log.Printf("warning: could not write PID file: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("shutting down...")
		srv.Shutdown()
		removePID()
		os.Exit(0)
	}()

	fmt.Print(`
 __   __  ___  _  _
 \ \ / / | __|(_)| |
  \ V /  | _| | || |
   \_/   |___||_||_|
`)
	log.Printf("veil started — DNS on %s", cfg.DNSListen)
	log.Println("tip: for full protection, disable DNS-over-HTTPS in your browser")

	if err := srv.Start(); err != nil {
		log.Fatalf("DNS server error: %v", err)
	}
}

func cmdStop() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("veil is not running (no PID file found)")
		os.Exit(1)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("could not find process %d\n", pid)
		os.Exit(1)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Printf("could not stop veil (pid %d): %v\n", pid, err)
		os.Exit(1)
	}

	removePID()
	fmt.Println("veil stopped")
}

func cmdStatus() {
	pid, err := readPID()
	if err != nil {
		fmt.Println("veil is not running")
		return
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Println("veil is not running (stale PID file)")
		removePID()
		return
	}

	if err := proc.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("veil is not running (stale PID file)")
		removePID()
		return
	}

	fmt.Printf("veil is running (pid %d)\n", pid)
}

func pidPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.veil/veil.pid"
}

func writePID() error {
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())), 0644)
}

func readPID() (int, error) {
	data, err := os.ReadFile(pidPath())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(string(data))
}

func removePID() {
	os.Remove(pidPath())
}
