package dns

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/0xr3ngar/veil/internal/blocker"
	"github.com/0xr3ngar/veil/internal/quotes"
	mdns "github.com/miekg/dns"
)

type Server struct {
	blocker     *blocker.Blocker
	upstreamDNS string
	redirectIP  net.IP
	listenAddr  string
	client      *mdns.Client

	udpServer *mdns.Server
	tcpServer *mdns.Server

	mu            sync.RWMutex
	redirectCache net.IP
	cacheExpiry   time.Time
}

func NewServer(b *blocker.Blocker, listenAddr, upstreamDNS, redirectTo string) *Server {
	s := &Server{
		blocker:     b,
		upstreamDNS: upstreamDNS,
		listenAddr:  listenAddr,
		client:      &mdns.Client{Timeout: 5 * time.Second},
	}

	if ip := net.ParseIP(redirectTo); ip != nil {
		s.redirectIP = ip
	} else {
		s.resolveRedirect(redirectTo)
	}

	return s
}

func (s *Server) resolveRedirect(domain string) {
	if !strings.HasSuffix(domain, ".") {
		domain += "."
	}
	msg := new(mdns.Msg)
	msg.SetQuestion(domain, mdns.TypeA)
	resp, _, err := s.client.Exchange(msg, s.upstreamDNS)
	if err != nil {
		log.Printf("failed to resolve redirect target %s: %v", domain, err)
		s.redirectIP = net.ParseIP("127.0.0.1")
		return
	}
	for _, ans := range resp.Answer {
		if a, ok := ans.(*mdns.A); ok {
			s.mu.Lock()
			s.redirectCache = a.A
			s.cacheExpiry = time.Now().Add(5 * time.Minute)
			s.redirectIP = a.A
			s.mu.Unlock()
			return
		}
	}
	s.redirectIP = net.ParseIP("127.0.0.1")
}

func (s *Server) getRedirectIP() net.IP {
	s.mu.RLock()
	ip := s.redirectIP
	expired := !s.cacheExpiry.IsZero() && time.Now().After(s.cacheExpiry)
	s.mu.RUnlock()

	if expired {
		go s.resolveRedirect(s.redirectIP.String())
	}
	return ip
}

func (s *Server) handleDNS(w mdns.ResponseWriter, r *mdns.Msg) {
	if len(r.Question) == 0 {
		mdns.HandleFailed(w, r)
		return
	}

	q := r.Question[0]
	domain := strings.TrimSuffix(q.Name, ".")

	if s.blocker.IsBlocked(domain) {
		s.blocker.TotalBlocked.Add(1)

		clientIP := ""
		if addr, ok := w.RemoteAddr().(*net.UDPAddr); ok {
			clientIP = addr.IP.String()
		} else if addr, ok := w.RemoteAddr().(*net.TCPAddr); ok {
			clientIP = addr.IP.String()
		}
		s.blocker.LogBlock(domain, clientIP)

		msg := new(mdns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true

		if q.Qtype == mdns.TypeA || q.Qtype == mdns.TypeANY {
			redirectIP := s.getRedirectIP()
			msg.Answer = append(msg.Answer, &mdns.A{
				Hdr: mdns.RR_Header{
					Name:   q.Name,
					Rrtype: mdns.TypeA,
					Class:  mdns.ClassINET,
					Ttl:    60,
				},
				A: redirectIP,
			})
		}

		w.WriteMsg(msg)
		q := quotes.Random()
		log.Printf("BLOCKED: %s -> %s | \"%s\" — %s", domain, s.getRedirectIP(), q.Text, q.Source)
		return
	}

	s.blocker.TotalAllowed.Add(1)

	resp, _, err := s.client.Exchange(r, s.upstreamDNS)
	if err != nil {
		log.Printf("upstream DNS error for %s: %v", domain, err)
		mdns.HandleFailed(w, r)
		return
	}
	w.WriteMsg(resp)
}

func (s *Server) Start() error {
	mux := mdns.NewServeMux()
	mux.HandleFunc(".", s.handleDNS)

	errCh := make(chan error, 2)

	s.udpServer = &mdns.Server{
		Addr:    s.listenAddr,
		Net:     "udp",
		Handler: mux,
	}

	s.tcpServer = &mdns.Server{
		Addr:    s.listenAddr,
		Net:     "tcp",
		Handler: mux,
	}

	go func() {
		log.Printf("DNS server (UDP) listening on %s", s.listenAddr)
		errCh <- s.udpServer.ListenAndServe()
	}()

	go func() {
		log.Printf("DNS server (TCP) listening on %s", s.listenAddr)
		errCh <- s.tcpServer.ListenAndServe()
	}()

	return <-errCh
}

func (s *Server) Shutdown() {
	if s.udpServer != nil {
		s.udpServer.Shutdown()
	}
	if s.tcpServer != nil {
		s.tcpServer.Shutdown()
	}
}
