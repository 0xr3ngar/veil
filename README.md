# Veil

A DNS-level domain blocker that intercepts every request before it leaves your machine. If it's on the list, it never gets through.

My first attempt at building something like this, definitely not production ready but works great as a personal tool.

## Quick Start

```bash
go build -o veil ./cmd/veil
sudo ./veil start
sudo networksetup -setdnsservers Wi-Fi 127.0.0.1
```

## Install as Startup Service

```bash
sudo cp ./veil /usr/local/bin/veil
sudo /usr/local/bin/veil install
```

## Commands

```
veil start              Start DNS proxy + web UI
veil stop               Stop the daemon
veil status             Show running status
veil block <domain>     Block a domain
veil allow <domain>     Whitelist a domain
veil unblock <domain>   Request unblock (24h cooldown)
veil lock --duration 7d Lock config for 7 days
veil lock --status      Check lock timer
veil list               Show all blocked domains
veil update             Refresh the OISD adult list
veil install            Install as startup service (macOS)
veil uninstall          Remove startup service
```

## Web Dashboard

`http://127.0.0.1:6144`

## How It Works

DNS queries go to `127.0.0.1:53`. Blocked domains return `127.0.0.1`. Everything else forwards to `8.8.8.8`. Config lives at `~/.veil/config.json`.
