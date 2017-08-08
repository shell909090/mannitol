package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/http2"

	"github.com/miekg/dns"
)

func ParseUint(s string) (n uint64) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		log.Fatal("ParseUint error.")
		return
	}
	return
}

type GoogleHttpsDns struct {
	baseurl   string
	transport http.RoundTripper
	subnet    string
}

func NewGoogleHttpsDns(myip string) (handler *GoogleHttpsDns, err error) {
	handler = &GoogleHttpsDns{
		baseurl:   "https://dns.google.com/resolve",
		transport: &http2.Transport{},
		subnet:    myip,
	}

	jsonresp, err := handler.QueryHttpsDNS("1", "www.google.com")
	if err != nil {
		log.Fatalf("warmup err: %s.", err.Error())
		return
	}

	for _, a := range jsonresp.Answer {
		log.Printf("google result: %s.", a.Data)
	}
	return
}

func (handler *GoogleHttpsDns) ServeDNS(w dns.ResponseWriter, quiz *dns.Msg) {
	jsonresp, err := handler.QueryHttpsDNS(
		fmt.Sprintf("%v", quiz.Question[0].Qtype), quiz.Question[0].Name)
	if err != nil {
		return
	}

	resp, err := jsonresp.TranslateAnswer(quiz)
	if err != nil {
		return
	}

	if Debug {
		log.Printf("resp: %s.", resp.String())
	}

	err = w.WriteMsg(resp)
	if err != nil {
		log.Printf("write failed: %s.", err.Error())
		return
	}
	return
}

func (handler *GoogleHttpsDns) QueryHttpsDNS(qtype, name string) (jsonresp *DNSMsg, err error) {
	req, err := http.NewRequest("GET", handler.baseurl, nil)
	if err != nil {
		log.Fatalf("fail to create request: %s.", err.Error())
		return
	}

	query := req.URL.Query()
	query.Add("name", name)
	query.Add("type", qtype)
	query.Add("edns_client_subnet", handler.subnet)
	req.URL.RawQuery = query.Encode()

	if Debug {
		log.Printf("url: %s.", req.URL.String())
	}

	resp, err := handler.transport.RoundTrip(req)
	if err != nil {
		log.Printf("https err:", err.Error())
		return
	}
	defer resp.Body.Close()

	jsonresp = &DNSMsg{}
	err = json.NewDecoder(resp.Body).Decode(&jsonresp)
	if err != nil {
		log.Printf("decode err:", err.Error())
		return
	}
	return
}

type DNSMsg struct {
	Status             int32         `json:"Status,omitempty"`
	TC                 bool          `json:"TC,omitempty"`
	RD                 bool          `json:"RD,omitempty"`
	RA                 bool          `json:"RA,omitempty"`
	AD                 bool          `json:"AD,omitempty"`
	CD                 bool          `json:"CD,omitempty"`
	Question           []DNSQuestion `json:"Question,omitempty"`
	Answer             []DNSRR       `json:"Answer,omitempty"`
	Authority          []DNSRR       `json:"Authority,omitempty"`
	Additional         []DNSRR       `json:"Additional,omitempty"`
	Edns_client_subnet string        `json:"edns_client_subnet,omitempty"`
	Comment            string        `json:"Comment,omitempty"`
}

type DNSQuestion struct {
	Name string `json:"name,omitempty"`
	Type int32  `json:"type,omitempty"`
}

type DNSRR struct {
	Name string `json:"name,omitempty"`
	Type int32  `json:"type,omitempty"`
	TTL  int32  `json:"TTL,omitempty"`
	Data string `json:"data,omitempty"`
}

func (msg *DNSMsg) TranslateAnswer(quiz *dns.Msg) (resp *dns.Msg, err error) {
	resp = &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Id:                 quiz.Id,
			Response:           (msg.Status == 0),
			Opcode:             dns.OpcodeQuery,
			Authoritative:      false,
			Truncated:          msg.TC,
			RecursionDesired:   msg.RD,
			RecursionAvailable: msg.RA,
			AuthenticatedData:  msg.AD,
			CheckingDisabled:   msg.CD,
			Rcode:              int(msg.Status),
		},
		Compress: quiz.Compress,
	}

	for idx, q := range msg.Question {
		resp.Question = append(resp.Question,
			dns.Question{
				q.Name,
				uint16(q.Type),
				quiz.Question[idx].Qclass,
			})
	}

	TranslateRRs(&msg.Answer, &resp.Answer)
	TranslateRRs(&msg.Authority, &resp.Ns)
	TranslateRRs(&msg.Additional, &resp.Extra)

	return
}

func TranslateRRs(jrs *[]DNSRR, rrs *[]dns.RR) {
	for _, jr := range *jrs {
		rr := jr.Translate()
		if rr != nil {
			*rrs = append(*rrs, rr)
		}
	}
}

func (jr *DNSRR) Translate() (rr dns.RR) {
	switch uint16(jr.Type) {
	case dns.TypeA:
		rr = &dns.A{
			A: net.ParseIP(jr.Data),
		}
	case dns.TypeNS:
		rr = &dns.NS{
			Ns: jr.Data,
		}
	case dns.TypeMD:
		rr = &dns.MD{
			Md: jr.Data,
		}
	case dns.TypeMF:
		rr = &dns.MF{
			Mf: jr.Data,
		}
	case dns.TypeCNAME:
		rr = &dns.CNAME{
			Target: jr.Data,
		}
	case dns.TypeSOA:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 7 {
			return
		}
		rr = &dns.SOA{
			Ns:      parts[0],
			Mbox:    parts[1],
			Serial:  uint32(ParseUint(parts[2])),
			Refresh: uint32(ParseUint(parts[3])),
			Retry:   uint32(ParseUint(parts[4])),
			Expire:  uint32(ParseUint(parts[5])),
			Minttl:  uint32(ParseUint(parts[6])),
		}
	case dns.TypeMB:
		rr = &dns.MB{
			Mb: jr.Data,
		}
	case dns.TypeMG:
		rr = &dns.MG{
			Mg: jr.Data,
		}
	case dns.TypeMR:
		rr = &dns.MR{
			Mr: jr.Data,
		}
	case dns.TypeNULL:
	case dns.TypePTR:
		rr = &dns.PTR{
			Ptr: jr.Data,
		}
	case dns.TypeHINFO:
	case dns.TypeMINFO:
	case dns.TypeMX:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 2 {
			return
		}
		rr = &dns.MX{
			Preference: uint16(ParseUint(parts[0])),
			Mx:         parts[1],
		}
	case dns.TypeTXT:
		rr = &dns.TXT{
			Txt: strings.Split(jr.Data, " "),
		}
	case dns.TypeRP:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 2 {
			return
		}
		rr = &dns.RP{
			Mbox: parts[0],
			Txt:  parts[1],
		}
	case dns.TypeAAAA:
		rr = &dns.AAAA{
			AAAA: net.ParseIP(jr.Data),
		}
	case dns.TypeSRV:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 4 {
			return
		}
		rr = &dns.SRV{
			Priority: uint16(ParseUint(parts[0])),
			Weight:   uint16(ParseUint(parts[1])),
			Port:     uint16(ParseUint(parts[2])),
			Target:   parts[3],
		}
	case dns.TypeSPF:
		rr = &dns.SPF{
			Txt: strings.Split(jr.Data, " "),
		}
	case dns.TypeDS:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 4 {
			return
		}
		rr = &dns.DS{
			KeyTag:     uint16(ParseUint(parts[0])),
			Algorithm:  uint8(ParseUint(parts[1])),
			DigestType: uint8(ParseUint(parts[2])),
			Digest:     parts[3],
		}
	case dns.TypeSSHFP:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 3 {
			return
		}
		rr = &dns.SSHFP{
			Algorithm:   uint8(ParseUint(parts[0])),
			Type:        uint8(ParseUint(parts[1])),
			FingerPrint: parts[2],
		}
	case dns.TypeRRSIG:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 9 {
			return
		}
		rrsig := &dns.RRSIG{
			Algorithm:  uint8(ParseUint(parts[1])),
			Labels:     uint8(ParseUint(parts[2])),
			OrigTtl:    uint32(ParseUint(parts[3])),
			Expiration: uint32(ParseUint(parts[4])),
			Inception:  uint32(ParseUint(parts[5])),
			KeyTag:     uint16(ParseUint(parts[6])),
			SignerName: parts[7],
			Signature:  parts[8],
		}
		var ok bool
		if rrsig.TypeCovered, ok = dns.StringToType[strings.ToUpper(parts[0])]; !ok {
			return
		}
		rr = rrsig
	case dns.TypeNSEC:
		nsec := &dns.NSEC{}
		parts := strings.Split(jr.Data, " ")
		nsec.NextDomain = parts[0]
		for _, d := range parts[1:] {
			if typeBit, ok := dns.StringToType[strings.ToUpper(d)]; ok {
				nsec.TypeBitMap = append(nsec.TypeBitMap, typeBit)
			}
		}
		rr = nsec
	case dns.TypeDNSKEY:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 4 {
			return
		}
		rr = &dns.DNSKEY{
			Flags:     uint16(ParseUint(parts[0])),
			Protocol:  uint8(ParseUint(parts[1])),
			Algorithm: uint8(ParseUint(parts[2])),
			PublicKey: parts[3],
		}
	case dns.TypeNSEC3:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 7 {
			return
		}
		nsec3 := &dns.NSEC3{
			Hash:       uint8(ParseUint(parts[0])),
			Flags:      uint8(ParseUint(parts[1])),
			Iterations: uint16(ParseUint(parts[2])),
			SaltLength: uint8(ParseUint(parts[3])),
			Salt:       parts[4],
			HashLength: uint8(ParseUint(parts[5])),
			NextDomain: parts[6],
		}
		for _, d := range parts[7:] {
			if t, ok := dns.StringToType[strings.ToUpper(d)]; ok {
				nsec3.TypeBitMap = append(nsec3.TypeBitMap, t)
			}
		}
		rr = nsec3
	case dns.TypeNSEC3PARAM:
		parts := strings.Split(jr.Data, " ")
		if len(parts) < 5 {
			return
		}
		rr = &dns.NSEC3PARAM{
			Hash:       uint8(ParseUint(parts[0])),
			Flags:      uint8(ParseUint(parts[1])),
			Iterations: uint16(ParseUint(parts[2])),
			SaltLength: uint8(ParseUint(parts[3])),
			Salt:       parts[4],
		}
	}
	hdr := &dns.RR_Header{
		Name:     jr.Name,
		Rrtype:   uint16(jr.Type),
		Ttl:      uint32(jr.TTL),
		Class:    dns.ClassINET,
		Rdlength: uint16(len(jr.Data)),
	}
	*(rr.Header()) = *hdr
	return
}
