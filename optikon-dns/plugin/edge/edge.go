package edge

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
	ot "github.com/opentracing/opentracing-go"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
)

const (
	pluginName              = "edge"
	defaultSvcReadInterval  = 2 * time.Second
	defaultSvcPushInterval  = 3 * time.Second
	defaultExpire           = 10 * time.Second
	defaultMaxUpstreamFails = 2
	maxUpstreams            = 15
)

// EdgeSite is a wrapper for all information needed about edge sites.
type EdgeSite struct {
	IP        net.IP `json:"ip"`
	GeoCoords *Point `json:"coords"`
}

// Edge encapsulates all edge plugin state.
type Edge struct {

	// Next is a reference to the next plugin in the CoreDNS plugin chain.
	Next plugin.Handler

	// Table stores the service->[]edgesite mappings for this and all
	// downstream edge sites.
	table *ConcurrentServiceTable

	// Clientset is a reference to in-cluster Kubernetes API.
	clientset *kubernetes.Clientset

	// IP is the public IP address of this cluster.
	ip net.IP

	// The geo coordinates of this cluster.
	geoCoords *Point

	// The interval for reading and pushing locally running Kubernetes services.
	svcReadInterval time.Duration
	svcPushInterval time.Duration

	// A channel for halting the service-reading process.
	svcReadChan chan struct{}

	// A server for receiving table updates from downstream edge sites.
	server *http.Server

	// The set of services currently running at this edge site.
	services *ConcurrentSet

	// The set of upstream proxies for forwarding requests.
	proxies []*Proxy

	// The policy for selecting the next upstream.
	policy Policy

	// The duration between proxy healthchecks.
	healthCheckInterval time.Duration

	// The base domain to match requests against.
	baseDomain string

	// The list of ignored IPs.
	ignored []string

	// The TLS configs for forwarding requests.
	tlsConfig     *tls.Config
	tlsServerName string

	// The maximum number of allowable failures before giving up forwarding.
	maxUpstreamFails uint32

	// The duration before expiring cached connections.
	expire time.Duration

	// Forces TCP forwarding even when the initial request was UDP.
	forceTCP bool
}

// New returns a new Edge instance.
func New() *Edge {
	return &Edge{
		maxUpstreamFails:    defaultMaxUpstreamFails,
		tlsConfig:           new(tls.Config),
		expire:              defaultExpire,
		policy:              new(random),
		baseDomain:          ".",
		healthCheckInterval: healthCheckDuration,
		svcReadInterval:     defaultSvcReadInterval,
		svcPushInterval:     defaultSvcPushInterval,
		table:               NewConcurrentServiceTable(),
		services:            NewConcurrentSet(),
	}
}

// Name implements the plugin.Handler interface.
func (e *Edge) Name() string { return pluginName }

// NumUpstreams returns the number of upstream proxies.
func (e *Edge) NumUpstreams() int { return len(e.proxies) }

// ServeDNS implements the plugin.Handler interface.
//
// Control flow: First determine if the request is invalid or blacklisted. If it
// is, then fall through to the next plugin. If not, then determine if the
// request has an Extra LOC record, meaning it was forwarded from a downstream
// edge plugin. If no LOC is found, it must be from a client. In that case,
// check if the requested service is running locally. If it is, return my IP.
// Otherwise, if a LOC was found, try to check my local table to see if I have
// a list of edge sites running the requested service. If I do, then determine
// the edge site closest to the location in LOC. If no LOC was found, simply
// try to find the service running closest to my location. If no entries can be
// found in my table for the requested service, then inject my location in a
// LOC record, and forward the request up to one of my upstreams. Whatever
// response they give me, I will return back to the client unmodified. Lastly,
// if I have no upstreams to foward to, fall through to the `proxy` plugin to
// handle this request.
func (e *Edge) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) (int, error) {

	// Encapsolate the state of the request and response.
	state := request.Request{W: w, Req: r}

	// If the request is invalid or should be ignored, fallthrough to the next plugin.
	if !e.match(state) {
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	// Declare the response we want to send back.
	res := new(dns.Msg)

	// Parse out (and remove) the LOC field from the request, if one exists.
	loc, locFound := extractLocationRecord(r)

	// Parse the target domain out of the request (NOTE: This will always have
	// a trailing dot.)
	requestedService := ServiceDNS(trimTrailingDot(state.Name()))

	// Determine if the requested service is running locally and write a reply
	// with my ip if it is.
	if !locFound && e.services.Contains(requestedService) {
		writeAuthoritativeResponse(res, &state, e.ip)
		log.Infof("requested service %s found running locally. returning my ip\n", requestedService)
		return dns.RcodeSuccess, nil
	}

	// Determine if there is another edge site that I know of that is running
	// the requested service. If there is, redirect to the closest.
	edgeSites, entryFound := e.table.Lookup(requestedService)
	if entryFound && len(edgeSites) > 0 {
		var closest net.IP
		if locFound {
			closest = findClosestToPoint(edgeSites, loc)
		} else {
			closest = findClosestToPoint(edgeSites, e.geoCoords)
		}
		writeAuthoritativeResponse(res, &state, closest)
		log.Infof("requested service %s found in table. returning its IP: %s", requestedService, closest.String())
		return dns.RcodeSuccess, nil
	}

	// If we have no upstream proxies to forward to, fallthrough to the
	// `proxy` plugin.
	if e.NumUpstreams() == 0 {
		log.Infoln("no upstream proxies to resolve request. falling through to `proxy` plugin")
		return plugin.NextOrFailure(e.Name(), e.Next, ctx, w, r)
	}

	// Inject my location as a LOC record in the Extra fields of the message.
	insertLocationRecord(r, e.geoCoords)

	// Forward the request to one of the upstream proxies.
	fails := 0
	var span, child ot.Span
	var upstreamErr error
	span = ot.SpanFromContext(ctx)
	for _, proxy := range e.list() {

		if proxy.Down(e.maxUpstreamFails) {
			fails++
			if fails < len(e.proxies) {
				continue
			}
			// All upstream proxies are dead, assume healtcheck is completely broken and randomly
			// select an upstream to connect to.
			r := new(random)
			proxy = r.List(e.proxies)[0]
		}

		if span != nil {
			child = span.Tracer().StartSpan("connect", ot.ChildOf(span.Context()))
			ctx = ot.ContextWithSpan(ctx, child)
		}

		res = new(dns.Msg)
		var err error
		var stop bool
		for {
			res, err = proxy.connect(ctx, state, e.forceTCP, true)
			if err != nil && err == io.EOF && !stop { // Remote side closed conn, can only happen with TCP.
				stop = true
				continue
			}
			break
		}

		if child != nil {
			child.Finish()
		}

		res, err = truncated(res, err)
		upstreamErr = err

		if err != nil {
			// Kick off health check to see if *our* upstream is broken.
			if e.maxUpstreamFails != 0 {
				proxy.Healthcheck()
			}
			if fails < len(e.proxies) {
				continue
			}
			break
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(res) {
			formerr := state.ErrorMessage(dns.RcodeFormatError)
			w.WriteMsg(formerr)
			return dns.RcodeSuccess, nil
		}

		// Compress the return message.
		res.Compress = true

		// When using force_tcp the upstream can send a message that is too big for
		// the udp buffer, hence we need to truncate the message to at least make it
		// fit the udp buffer.
		res, _ = state.Scrub(res)

		// Write the response message.
		w.WriteMsg(res)

		return dns.RcodeSuccess, nil
	}

	if upstreamErr != nil {
		return dns.RcodeServerFailure, upstreamErr
	}

	return dns.RcodeServerFailure, errNoHealthy
}

// Write the given IP address as an Authoritative Answer to the request.
func writeAuthoritativeResponse(res *dns.Msg, state *request.Request, ip net.IP) {

	// Set the reply to the given request.
	res.SetReply(state.Req)

	// Make the answer Authoritative and compressed.
	res.Authoritative, res.Compress = true, true

	// Add the IP address to the Answer field.
	var rr dns.RR
	switch state.Family() {
	case 1:
		rr = new(dns.A)
		rr.(*dns.A).Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeA, Class: state.QClass()}
		rr.(*dns.A).A = ip.To4()
	case 2:
		rr = new(dns.AAAA)
		rr.(*dns.AAAA).Hdr = dns.RR_Header{Name: state.QName(), Rrtype: dns.TypeAAAA, Class: state.QClass()}
		rr.(*dns.AAAA).AAAA = ip
	}
	res.Answer = []dns.RR{rr}

	// Write the message.
	state.W.WriteMsg(res)
}

// Determines the IP address of the edge site closest to the given Point.
func findClosestToPoint(edgeSiteSet Set, p *Point) net.IP {
	var closest net.IP
	var minDist float64
	first := true
	for val := range edgeSiteSet {
		edgeSite := val.(EdgeSite)
		dist := p.GreatCircleDistance(edgeSite.GeoCoords)
		if first || dist < minDist {
			closest = edgeSite.IP
			minDist = dist
			first = false
		}
	}
	return closest
}

// Removes the root domain from a DNS address.
func trimTrailingDot(s string) string {
	if s == "" || s[len(s)-1] != '.' {
		return s
	}
	return s[:(len(s) - 1)]
}

// Returns true if the request domain should be accepted or not.
func (e *Edge) match(state request.Request) bool {
	baseDomain := e.baseDomain
	if !plugin.Name(baseDomain).Matches(state.Name()) || !e.isAllowedDomain(state.Name()) {
		return false
	}
	return true
}

// Determines whether or not the given domain name should be ignored.
func (e *Edge) isAllowedDomain(name string) bool {
	if dns.Name(name) == dns.Name(e.baseDomain) {
		return true
	}
	for _, ignore := range e.ignored {
		if plugin.Name(ignore).Matches(name) {
			return false
		}
	}
	return true
}

// List returns a set of proxies to be used for this client depending on the policy in e.
func (e *Edge) list() []*Proxy { return e.policy.List(e.proxies) }
