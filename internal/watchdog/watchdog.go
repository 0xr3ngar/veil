package watchdog

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Watchdog struct {
	blockedDomains []string
	redirectIP     string
	veilBinary     string
	checkInterval  time.Duration
	stopCh         chan struct{}
}

func New(blockedDomains []string, redirectIP, veilBinary string) *Watchdog {
	return &Watchdog{
		blockedDomains: blockedDomains,
		redirectIP:     redirectIP,
		veilBinary:     veilBinary,
		checkInterval:  10 * time.Second,
		stopCh:         make(chan struct{}),
	}
}

func (w *Watchdog) Start() {
	go w.monitorDNSSettings()
	go w.monitorHostsFile()
	go w.monitorProcess()
}

func (w *Watchdog) Stop() {
	close(w.stopCh)
}

func (w *Watchdog) monitorProcess() {
	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			if !isDNSResponding() {
				log.Println("[watchdog] DNS server not responding, attempting restart...")
				w.restartVeil()
			}
		}
	}
}

func isDNSResponding() bool {
	conn, err := net.DialTimeout("udp", "127.0.0.1:53", 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (w *Watchdog) restartVeil() {
	if w.veilBinary == "" {
		return
	}
	cmd := exec.Command(w.veilBinary, "start")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("[watchdog] failed to restart veil: %v", err)
	}
}

func (w *Watchdog) monitorDNSSettings() {
	if runtime.GOOS != "darwin" {
		return
	}

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			if !isDNSSetCorrectly() {
				log.Println("[watchdog] system DNS changed, resetting to 127.0.0.1...")
				resetDNS()
			}
		}
	}
}

func isDNSSetCorrectly() bool {
	ifaces := []string{"Wi-Fi", "Ethernet"}
	for _, iface := range ifaces {
		out, err := exec.Command("networksetup", "-getdnsservers", iface).Output()
		if err != nil {
			continue
		}
		if strings.Contains(string(out), "127.0.0.1") {
			return true
		}
	}
	return false
}

func resetDNS() {
	ifaces := []string{"Wi-Fi", "Ethernet"}
	for _, iface := range ifaces {
		exec.Command("networksetup", "-setdnsservers", iface, "127.0.0.1").Run()
	}
}

func (w *Watchdog) monitorHostsFile() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-w.stopCh:
			return
		case <-ticker.C:
			w.ensureHostsEntries()
		}
	}
}

func (w *Watchdog) ensureHostsEntries() {
	hostsPath := "/etc/hosts"
	existing := readHostsVeilEntries(hostsPath)

	missing := make([]string, 0)
	for _, domain := range w.blockedDomains {
		if _, ok := existing[domain]; !ok {
			missing = append(missing, domain)
		}
	}

	if len(missing) == 0 {
		return
	}

	log.Printf("[watchdog] adding %d missing entries to /etc/hosts", len(missing))
	w.writeHostsEntries(hostsPath, missing)
}

func readHostsVeilEntries(path string) map[string]struct{} {
	entries := make(map[string]struct{})
	f, err := os.Open(path)
	if err != nil {
		return entries
	}
	defer f.Close()

	inVeilBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "# BEGIN VEIL" {
			inVeilBlock = true
			continue
		}
		if line == "# END VEIL" {
			inVeilBlock = false
			continue
		}
		if inVeilBlock {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				entries[parts[1]] = struct{}{}
			}
		}
	}
	return entries
}

func (w *Watchdog) writeHostsEntries(hostsPath string, domains []string) {
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	skip := false
	for _, line := range lines {
		if line == "# BEGIN VEIL" {
			skip = true
			continue
		}
		if line == "# END VEIL" {
			skip = false
			continue
		}
		if !skip {
			newLines = append(newLines, line)
		}
	}

	newLines = append(newLines, "# BEGIN VEIL")
	for _, domain := range w.blockedDomains {
		newLines = append(newLines, fmt.Sprintf("%s %s", w.redirectIP, domain))
	}
	newLines = append(newLines, "# END VEIL")

	atomicWriteFile(hostsPath, []byte(strings.Join(newLines, "\n")), 0644)
}

func WriteInitialHosts(domains []string, redirectIP string) error {
	hostsPath := "/etc/hosts"
	content, err := os.ReadFile(hostsPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(content), "\n")
	var newLines []string
	skip := false
	for _, line := range lines {
		if line == "# BEGIN VEIL" {
			skip = true
			continue
		}
		if line == "# END VEIL" {
			skip = false
			continue
		}
		if !skip {
			newLines = append(newLines, line)
		}
	}

	newLines = append(newLines, "# BEGIN VEIL")
	for _, domain := range domains {
		newLines = append(newLines, fmt.Sprintf("%s %s", redirectIP, domain))
	}
	newLines = append(newLines, "# END VEIL")

	return atomicWriteFile(hostsPath, []byte(strings.Join(newLines, "\n")), 0644)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".veil.tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func InstallLaunchDaemon(veilBinary string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("launchd is only available on macOS")
	}

	absPath, err := filepath.Abs(veilBinary)
	if err != nil {
		return fmt.Errorf("resolving binary path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("binary not found at %s: %w", absPath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory, not a binary", absPath)
	}
	veilBinary = absPath

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.veil.dns</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/var/log/veil.log</string>
    <key>StandardErrorPath</key>
    <string>/var/log/veil.log</string>
</dict>
</plist>`, veilBinary)

	daemonPath := "/Library/LaunchDaemons/com.veil.dns.plist"
	if err := os.WriteFile(daemonPath, []byte(plist), 0600); err != nil {
		return fmt.Errorf("writing launch daemon: %w", err)
	}

	return exec.Command("launchctl", "load", daemonPath).Run()
}

func UninstallLaunchDaemon() error {
	daemonPath := filepath.Join("/Library/LaunchDaemons", "com.veil.dns.plist")
	exec.Command("launchctl", "unload", daemonPath).Run()
	return os.Remove(daemonPath)
}
