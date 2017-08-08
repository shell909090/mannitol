package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/miekg/dns"
)

func appendEdns0Subnet(msg *dns.Msg, addr net.IP) {
	newOpt := true
	var o *dns.OPT
	for _, v := range msg.Extra {
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
	e := &dns.EDNS0_SUBNET{
		Code:        dns.EDNS0SUBNET,
		SourceScope: 0,
		Address:     addr,
	}
	if addr.To4() == nil {
		e.Family = 2 // IP6
		e.SourceNetmask = net.IPv6len * 8
	} else {
		e.Family = 1 // IP4
		e.SourceNetmask = net.IPv4len * 8
	}
	o.Option = append(o.Option, e)
	if newOpt {
		msg.Extra = append(msg.Extra, o)
	}
}

type Forwarder struct {
	client  *dns.Client
	servers []string
	subnet  net.IP
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
	}
	return
}

func (handler *Forwarder) ServeDNS(w dns.ResponseWriter, quiz *dns.Msg) {
	var resp *dns.Msg
	var err error
	var dbglog bytes.Buffer

	if Debug {
		fmt.Fprintf(&dbglog, "query: %s ", quiz.Question[0].Name)
	}

	appendEdns0Subnet(quiz, handler.subnet)

	for _, srv := range handler.servers {
		if Debug {
			fmt.Fprintf(&dbglog, "srv: %s ", srv)
		}
		resp, _, err = handler.client.Exchange(quiz, srv)
		if err == nil {
			break
		}
	}
	if err != nil {
		log.Printf("get upstream failed: %s.", err.Error())
		return
	}

	if Debug {
		fmt.Fprintf(&dbglog, "result: ")
		for _, ans := range resp.Answer {
			if a, ok := ans.(*dns.A); ok {
				fmt.Fprintf(&dbglog, "%s ", a.A.String())
			}
		}
		log.Print(dbglog.String())
	}

	err = w.WriteMsg(resp)
	if err != nil {
		log.Printf("write failed: %s.", err.Error())
		return
	}
	return
}
