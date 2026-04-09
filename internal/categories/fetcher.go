package categories

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const OISDAdultURL = "https://nsfw.oisd.nl/"

func FetchAdultList() ([]string, error) {
	resp, err := http.Get(OISDAdultURL)
	if err != nil {
		return nil, fmt.Errorf("fetching OISD list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OISD returned status %d", resp.StatusCode)
	}

	return parseAdblockList(resp.Body)
}

func parseAdblockList(r io.Reader) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "||") || !strings.HasSuffix(line, "^") {
			continue
		}
		domain := line[2 : len(line)-1]
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains, scanner.Err()
}

func CachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "lists", "adult.txt")
}

func SaveCache(domains []string) error {
	path := CachePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, d := range domains {
		w.WriteString(d)
		w.WriteByte('\n')
	}
	return w.Flush()
}

func LoadCache() ([]string, error) {
	f, err := os.Open(CachePath())
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var domains []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			domains = append(domains, line)
		}
	}
	return domains, scanner.Err()
}

func LoadAdultList() ([]string, error) {
	domains, err := LoadCache()
	if err == nil && len(domains) > 0 {
		return domains, nil
	}

	domains, err = FetchAdultList()
	if err != nil {
		return nil, err
	}

	_ = SaveCache(domains)
	return domains, nil
}
