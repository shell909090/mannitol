package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

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

type Forwarder struct {
	client  *dns.Client
	servers []string
	subnet  net.IP
	debug   bool
}

func NewForwarder(myip string, upstream string) (handler *Forwarder, err error) {
	mynet := net.ParseIP(myip)
	if mynet == nil {
		err = ErrParseIP
		log.Fatal(err.Error())
		return
	}

	handler = &Forwarder{
		client:  &dns.Client{},
		servers: strings.Split(upstream, ","),
		subnet:  mynet,
		debug:   true,
	}
	return
}

func (handler *Forwarder) ServeDNS(w dns.ResponseWriter, q *dns.Msg) {
	var r *dns.Msg
	var err error
	var dbglog bytes.Buffer

	if handler.debug {
		fmt.Fprintf(&dbglog, "query: %s ", q.Question[0].Name)
	}

	appendEdns0Subnet(q, handler.subnet)

	for _, srv := range handler.servers {
		if handler.debug {
			fmt.Fprintf(&dbglog, "srv: %s ", srv)
		}
		r, _, err = handler.client.Exchange(q, srv)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("get upstream failed: %s.", err.Error())
		return
	}

	if handler.debug {
		fmt.Fprintf(&dbglog, "result: ")
		for _, ans := range r.Answer {
			if a, ok := ans.(*dns.A); ok {
				fmt.Fprintf(&dbglog, "%s ", a.A.String())
			}
		}
		log.Print(dbglog.String())
	}

	err = w.WriteMsg(r)
	if err != nil {
		log.Printf("write failed: %s.", err.Error())
		return
	}
	return
}
