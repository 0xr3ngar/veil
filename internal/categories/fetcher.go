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

const (
	OISDAdultURL        = "https://nsfw.oisd.nl/"
	StevenBlackGambling = "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/gambling-only/hosts"
	StevenBlackFakeNews = "https://raw.githubusercontent.com/StevenBlack/hosts/master/alternates/fakenews-only/hosts"
)

type listFormat int

const (
	formatAdblock listFormat = iota
	formatHosts
)

type externalList struct {
	URL    string
	Cache  string
	Format listFormat
}

var externalLists = map[string]externalList{
	"adult": {
		URL:    OISDAdultURL,
		Cache:  "adult.txt",
		Format: formatAdblock,
	},
	"gambling": {
		URL:    StevenBlackGambling,
		Cache:  "gambling.txt",
		Format: formatHosts,
	},
	"fakenews": {
		URL:    StevenBlackFakeNews,
		Cache:  "fakenews.txt",
		Format: formatHosts,
	},
}

func fetchList(url string) (io.ReadCloser, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}
	return resp.Body, nil
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

func parseHostsList(r io.Reader) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && (parts[0] == "0.0.0.0" || parts[0] == "127.0.0.1") {
			domain := strings.ToLower(parts[1])
			if domain != "localhost" && domain != "" {
				domains = append(domains, domain)
			}
		}
	}
	return domains, scanner.Err()
}

func cachePath(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".veil", "lists", name)
}

func saveCache(name string, domains []string) error {
	path := cachePath(name)
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

func loadCache(name string) ([]string, error) {
	f, err := os.Open(cachePath(name))
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

func FetchExternalList(name string) ([]string, error) {
	list, ok := externalLists[name]
	if !ok {
		return nil, fmt.Errorf("unknown external list: %s", name)
	}

	body, err := fetchList(list.URL)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	var domains []string
	switch list.Format {
	case formatAdblock:
		domains, err = parseAdblockList(body)
	case formatHosts:
		domains, err = parseHostsList(body)
	}
	if err != nil {
		return nil, err
	}

	_ = saveCache(list.Cache, domains)
	return domains, nil
}

func LoadExternalList(name string) ([]string, error) {
	list, ok := externalLists[name]
	if !ok {
		return nil, fmt.Errorf("unknown external list: %s", name)
	}

	domains, err := loadCache(list.Cache)
	if err == nil && len(domains) > 0 {
		return domains, nil
	}

	return FetchExternalList(name)
}

func IsExternalList(name string) bool {
	_, ok := externalLists[name]
	return ok
}

func ExternalListNames() []string {
	names := make([]string, 0, len(externalLists))
	for name := range externalLists {
		names = append(names, name)
	}
	return names
}
