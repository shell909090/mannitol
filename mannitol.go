package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/miekg/dns"
)

var (
	ErrParseIP = errors.New("can't get myip.")
	reip       = regexp.MustCompile("(?:[0-9]{1,3}\\.){3}[0-9]{1,3}")
	upstream   string
	myip       string
	listenaddr string
)

func init() {
	flag.StringVar(&upstream, "up", "8.8.8.8:53,8.8.4.4:53", "upstream server")
	flag.StringVar(&myip, "myip", "", "my ip address")
	flag.StringVar(&listenaddr, "listen", "127.0.0.1:5553", "listen address")
}

func getMyIp() (ip string, err error) {
	resp, err := http.Get("http://myip.ipip.net")
	if err != nil {
		log.Fatalf("get myip err: %s.", err.Error())
		return
	}

	p, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("myip read err: %s.", err.Error())
		return
	}

	ipstrs := reip.FindAllString(string(p), 1)
	if ipstrs == nil {
		err = ErrParseIP
		return
	}

	ip = ipstrs[0]
	return
}

func appendEdns0Subnet(m *dns.Msg, addr net.IP) {
	newOpt := true
	var o *dns.OPT
	for _, v := range m.Extra {
		if v.Header().Rrtype == dns.TypeOPT {
			o = v.(*dns.OPT)
			newOpt = false
			break
		}
	}
	if o == nil {
		o = new(dns.OPT)
		o.Hdr.Name = "."
		o.Hdr.Rrtype = dns.TypeOPT
	}
	e := new(dns.EDNS0_SUBNET)
	e.Code = dns.EDNS0SUBNET
	e.SourceScope = 0
	e.Address = addr
	if e.Address.To4() == nil {
		e.Family = 2 // IP6
		e.SourceNetmask = net.IPv6len * 8
	} else {
		e.Family = 1 // IP4
		e.SourceNetmask = net.IPv4len * 8
	}
	o.Option = append(o.Option, e)
	if newOpt {
		m.Extra = append(m.Extra, o)
	}
}

type QueryHandler struct {
	client  *dns.Client
	servers []string
	subnet  net.IP
}

func NewQueryHandler() (handler *QueryHandler, err error) {
	if myip == "" {
		myip, err = getMyIp()
		if err != nil {
			log.Fatal(err.Error())
			return
		}
		log.Printf("auto get myip: %s.", myip)
	}

	mynet := net.ParseIP(myip)
	if mynet == nil {
		err = ErrParseIP
		log.Fatal(err.Error())
		return
	}

	handler = &QueryHandler{
		client:  &dns.Client{},
		servers: strings.Split(upstream, ","),
		subnet:  mynet,
	}
	return
}

func (handler *QueryHandler) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	var r *dns.Msg
	var err error

	log.Printf("dns query for %s.", q.Question[0].Name)
	appendEdns0Subnet(q, handler.subnet)

	for _, srv := range handler.servers {
		log.Printf("query server: %s.", srv)
		r, _, err = handler.client.Exchange(q, srv)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("get upstream failed: %s.", err.Error())
		return
	}

	for _, ans := range r.Answer {
		if a, ok := ans.(*dns.A); ok {
			log.Printf("upstream result: %s.", a.A.String())
		}
	}

	err = w.WriteMsg(r)
	if err != nil {
		log.Printf("write failed: %s.", err.Error())
		return
	}
	return
}

func main() {
	flag.Parse()

	handler, err := NewQueryHandler()
	if err != nil {
		log.Fatalf(err.Error())
		return
	}

	server := &dns.Server{
		Addr:    listenaddr,
		Net:     "udp",
		Handler: handler,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Fatalf(err.Error())
		return
	}
}
