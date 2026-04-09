package categories

var SocialMedia = []string{
	"tiktok.com",
	"twitter.com",
	"x.com",
	"instagram.com",
	"facebook.com",
	"snapchat.com",
	"threads.net",
	"pinterest.com",
	"tumblr.com",
}

var Gambling = []string{
	"draftkings.com",
	"fanduel.com",
	"bet365.com",
	"pokerstars.com",
	"betway.com",
	"bovada.lv",
	"williamhill.com",
	"888poker.com",
	"unibet.com",
	"betfair.com",
}

var Streaming = []string{
	"twitch.tv",
	"netflix.com",
	"youtube.com",
	"hulu.com",
	"disneyplus.com",
	"primevideo.com",
	"crunchyroll.com",
}

var DoHBypass = []string{
	"mozilla.cloudflare-dns.com",
	"dns.google",
	"dns.google.com",
	"cloudflare-dns.com",
	"one.one.one.one",
	"doh.opendns.com",
	"dns.quad9.net",
	"doh.cleanbrowsing.org",
	"dns.adguard.com",
	"doh.dns.sb",
}

var All = map[string][]string{
	"social_media": SocialMedia,
	"gambling":     Gambling,
	"streaming":    Streaming,
	"doh_bypass":   DoHBypass,
}
