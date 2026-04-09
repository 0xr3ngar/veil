package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/0xr3ngar/veil/internal/api"
	"github.com/0xr3ngar/veil/internal/blocker"
	"github.com/0xr3ngar/veil/internal/categories"
	"github.com/0xr3ngar/veil/internal/config"
	vdns "github.com/0xr3ngar/veil/internal/dns"
	"github.com/0xr3ngar/veil/internal/lock"
	"github.com/0xr3ngar/veil/internal/watchdog"
	"github.com/0xr3ngar/veil/internal/webui"
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
	case "block":
		cmdBlock()
	case "allow":
		cmdAllow()
	case "unblock":
		cmdUnblock()
	case "cancel-unblock":
		cmdCancelUnblock()
	case "lock":
		cmdLock()
	case "list":
		cmdList()
	case "update":
		cmdUpdate()
	case "config":
		cmdConfig()
	case "install":
		cmdInstall()
	case "uninstall":
		cmdUninstall()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Usage: veil <command>

Commands:
  start            Start the DNS proxy and web UI
  stop             Stop the running daemon
  status           Show running status
  block <domain>   Add domain to blocklist
  allow <domain>   Add domain to whitelist
  unblock <domain> Request domain unblock (24h cooldown)
  cancel-unblock <domain>  Cancel pending unblock
  lock             Set or check time lock
  list             Show all blocked domains
  update           Re-download external blocklists
  config           Show current config
  install          Install as startup service (launchd)
  uninstall        Remove startup service`)
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

	a := api.New(b, cfg)
	webSrv := webui.NewServer(b, cfg, a)

	go func() {
		log.Printf("web UI listening on http://%s", cfg.APIListen)
		if err := http.ListenAndServe(cfg.APIListen, webSrv.Handler()); err != nil {
			log.Printf("web server error: %v", err)
		}
	}()

	go func() {
		log.Println("blocked page listening on http://127.0.0.1:80")
		if err := http.ListenAndServe("127.0.0.1:80", webSrv.BlockedHandler()); err != nil {
			log.Printf("blocked page server error: %v", err)
		}
	}()

	if err := vdns.DropPrivileges(); err != nil {
		log.Printf("warning: could not drop privileges: %v", err)
	}

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

	if lock.IsLocked() {
		fmt.Printf("lock: active (%s remaining)\n", formatDuration(lock.Remaining()))
	} else {
		fmt.Println("lock: inactive")
	}
}

func cmdBlock() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: veil block <domain>")
		os.Exit(1)
	}
	domain := strings.ToLower(strings.TrimSpace(os.Args[2]))

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	cfg.Update(func(c *config.Config) {
		for _, d := range c.CustomBlocked {
			if d == domain {
				return
			}
		}
		c.CustomBlocked = append(c.CustomBlocked, domain)
	})

	if err := cfg.Save(); err != nil {
		log.Fatalf("failed to save config: %v", err)
	}
	fmt.Printf("blocked: %s\n", domain)
}

func cmdAllow() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: veil allow <domain>")
		os.Exit(1)
	}

	if lock.IsLocked() {
		fmt.Fprintln(os.Stderr, "cannot add allowed domains while locked")
		os.Exit(1)
	}

	domain := strings.ToLower(strings.TrimSpace(os.Args[2]))

	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	cfg.Update(func(c *config.Config) {
		for _, d := range c.CustomAllowed {
			if d == domain {
				return
			}
		}
		c.CustomAllowed = append(c.CustomAllowed, domain)
	})

	if err := cfg.Save(); err != nil {
		log.Fatalf("failed to save config: %v", err)
	}
	fmt.Printf("allowed: %s\n", domain)
}

func cmdUnblock() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: veil unblock <domain>")
		os.Exit(1)
	}

	if lock.IsLocked() {
		fmt.Fprintln(os.Stderr, "cannot unblock while locked")
		os.Exit(1)
	}

	domain := strings.ToLower(strings.TrimSpace(os.Args[2]))
	pending, err := lock.RequestUnblock(domain)
	if err != nil {
		log.Fatalf("failed to request unblock: %v", err)
	}

	fmt.Printf("unblock request queued for %s\n", domain)
	fmt.Printf("will take effect at %s (in 24 hours)\n", pending.EffectAt.Format("Jan 2, 2006 15:04"))
	fmt.Printf("cancel with: veil cancel-unblock %s\n", domain)
}

func cmdCancelUnblock() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: veil cancel-unblock <domain>")
		os.Exit(1)
	}

	domain := strings.ToLower(strings.TrimSpace(os.Args[2]))
	if err := lock.CancelUnblock(domain); err != nil {
		log.Fatalf("failed to cancel unblock: %v", err)
	}
	fmt.Printf("cancelled unblock for %s\n", domain)
}

func cmdLock() {
	if len(os.Args) >= 3 && os.Args[2] == "--status" {
		if lock.IsLocked() {
			state, _ := lock.GetLock()
			fmt.Printf("locked until %s (%s remaining)\n",
				state.LockedUntil.Format("Jan 2, 2006 15:04"),
				formatDuration(lock.Remaining()))
		} else {
			fmt.Println("not locked")
		}
		return
	}

	duration := "7d"
	for i, arg := range os.Args {
		if arg == "--duration" && i+1 < len(os.Args) {
			duration = os.Args[i+1]
		}
	}

	d, err := parseDuration(duration)
	if err != nil {
		log.Fatalf("invalid duration %q: %v", duration, err)
	}

	if err := lock.SetLock(d); err != nil {
		log.Fatalf("failed to set lock: %v", err)
	}
	fmt.Printf("locked for %s (until %s)\n", duration, time.Now().Add(d).Format("Jan 2, 2006 15:04"))
}

func cmdList() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	fmt.Println("=== Categories ===")
	for name, enabled := range cfg.Categories {
		status := "OFF"
		if enabled {
			status = "ON"
		}
		count := len(categories.All[name])
		if name == "adult" {
			count = -1
		}
		if count >= 0 {
			fmt.Printf("  %-15s %s (%d domains)\n", name, status, count)
		} else {
			fmt.Printf("  %-15s %s (external list)\n", name, status)
		}
	}

	fmt.Println("\n=== Custom Blocked ===")
	if len(cfg.CustomBlocked) == 0 {
		fmt.Println("  (none)")
	}
	for _, d := range cfg.CustomBlocked {
		fmt.Printf("  %s\n", d)
	}

	fmt.Println("\n=== Custom Allowed ===")
	if len(cfg.CustomAllowed) == 0 {
		fmt.Println("  (none)")
	}
	for _, d := range cfg.CustomAllowed {
		fmt.Printf("  %s\n", d)
	}
}

func cmdUpdate() {
	for _, name := range categories.ExternalListNames() {
		fmt.Printf("updating %s list...\n", name)
		domains, err := categories.FetchExternalList(name)
		if err != nil {
			log.Printf("failed to fetch %s: %v", name, err)
			continue
		}
		fmt.Printf("  %s: %d domains cached\n", name, len(domains))
	}
}

func cmdConfig() {
	cfg, err := config.Load(config.DefaultPath())
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	fmt.Printf("config path: %s\n", config.DefaultPath())
	fmt.Printf("upstream DNS: %s\n", cfg.UpstreamDNS)
	fmt.Printf("redirect to: %s\n", cfg.RedirectTo)
	fmt.Printf("DNS listen: %s\n", cfg.DNSListen)
	fmt.Printf("API listen: %s\n", cfg.APIListen)
	fmt.Printf("log blocked: %v\n", cfg.LogBlocked)
}

func cmdInstall() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "install requires root — run with sudo")
		os.Exit(1)
	}

	binPath, err := os.Executable()
	if err != nil {
		log.Fatalf("could not determine binary path: %v", err)
	}

	if err := watchdog.InstallLaunchDaemon(binPath); err != nil {
		log.Fatalf("failed to install: %v", err)
	}
	fmt.Println("veil installed as startup service")
	fmt.Println("it will start automatically on boot and restart if killed")
	fmt.Printf("logs: /var/log/veil.log\n")
}

func cmdUninstall() {
	if os.Getuid() != 0 {
		fmt.Fprintln(os.Stderr, "uninstall requires root — run with sudo")
		os.Exit(1)
	}

	if err := watchdog.UninstallLaunchDaemon(); err != nil {
		log.Fatalf("failed to uninstall: %v", err)
	}
	fmt.Println("veil startup service removed")
}

func pidPath() string {
	home, _ := os.UserHomeDir()
	return home + "/.veil/veil.pid"
}

func writePID() error {
	return os.WriteFile(pidPath(), []byte(strconv.Itoa(os.Getpid())), 0600)
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

func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("too short")
	}
	num, err := strconv.Atoi(s[:len(s)-1])
	if err != nil {
		return 0, err
	}
	switch s[len(s)-1] {
	case 'd':
		return time.Duration(num) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(num) * 7 * 24 * time.Hour, nil
	case 'h':
		return time.Duration(num) * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit %c (use d/w/h)", s[len(s)-1])
	}
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("%dd %dh", days, hours)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
