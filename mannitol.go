package main

import (
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"

	"github.com/miekg/dns"
)

var (
	ErrParseIP = errors.New("can't get myip.")
	reip       = regexp.MustCompile("(?:[0-9]{1,3}\\.){3}[0-9]{1,3}")
	RunMode    string
	Upstream   string
	MyIP       string
	ListenAddr string
	Debug      = true
)

func init() {
	flag.StringVar(&RunMode, "mode", "https",
		"dns for forwarder, https for google https dns")
	flag.StringVar(&Upstream, "up", "8.8.8.8:53,8.8.4.4:53", "upstream server")
	flag.StringVar(&MyIP, "myip", "", "my ip address")
	flag.StringVar(&ListenAddr, "listen", "127.0.0.1:5553", "listen address")
}

func getMyIP() (ip string, err error) {
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

	return ipstrs[0], nil
}

func main() {
	var err error
	var handler dns.Handler

	flag.Parse()

	if MyIP == "" {
		MyIP, err = getMyIP()
		if err != nil {
			log.Fatal(err.Error())
			return
		}
		log.Printf("auto get myip: %s.", MyIP)
	}

	switch RunMode {
	case "dns":
		handler, err = NewForwarder(MyIP, Upstream)
	case "https":
		handler, err = NewGoogleHttpsDns(MyIP)
	default:
		log.Fatalf("unknown mode: %s.", RunMode)
		return
	}
	if err != nil {
		log.Fatalf(err.Error())
		return
	}

	server := &dns.Server{
		Addr:    ListenAddr,
		Net:     "udp",
		Handler: handler,
	}
	err = server.ListenAndServe()
	if err != nil {
		log.Fatalf(err.Error())
		return
	}
}
