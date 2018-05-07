/*
 * Copyright 2018 The CoreDNS Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may
 * not use this file except in compliance with the License. You may obtain
 * a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * NOTE: This software contains code derived from the Apache-licensed CoreDNS
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/setup.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/mholt/caddy"
	"github.com/sirupsen/logrus"

	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	pkgtls "github.com/coredns/coredns/plugin/pkg/tls"
)

// The global logger for this plugin.
var log *logrus.Logger

// Registers plugin and initializes logger.
func init() {

	// Initialize logger.
	log = logrus.New()
	log.Out = os.Stdout

	// Register plugin with caddy.
	caddy.RegisterPlugin(pluginName, caddy.Plugin{
		ServerType: "dns",
		Action:     setup,
	})
}

// Specifies everything to be run/configured before serving DNS queries.
func setup(c *caddy.Controller) error {

	// Parse the plugin arguments.
	e, err := parseEdge(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	// Make sure the max number of upstream proxies isn't exceeded.
	if e.NumUpstreams() > maxUpstreams {
		return plugin.Error(pluginName, fmt.Errorf("more than %d TOs configured: %d", maxUpstreams, e.NumUpstreams()))
	}

	// Convert the geographic lon-lat coordinates into a LOC record.
	e.locRR, err = convertPointToLOC(e.geoCoords)
	if err != nil {
		return err
	}

	// Add the plugin handler to the dnsserver.
	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		e.clientset, err = RegisterKubernetesClient()
		e.Next = next
		return e
	})
	if err != nil {
		return err
	}

	// Declare a startup routine.
	c.OnStartup(func() error {
		log.Infof("Starting %s plugin...", pluginName)
		return e.OnStartup()
	})

	// Declare a teardown routine.
	c.OnShutdown(func() error {
		log.Infof("Shutting down %s plugin...", pluginName)
		return e.OnShutdown()
	})

	return nil
}

// OnStartup starts reading/pushing services and listening for downstream
// table updates.
func (e *Edge) OnStartup() (err error) {
	e.startReadingServices()
	e.startListeningForTableUpdates()
	e.site = Site{
		IP:        e.ip,
		GeoCoords: e.geoCoords,
	}
	for _, p := range e.proxies {
		p.start(e.healthCheckInterval)
	}
	return nil
}

// OnShutdown stops all async processes.
func (e *Edge) OnShutdown() error {
	e.stopReadingServices()
	e.stopListeningForTableUpdates()
	for _, p := range e.proxies {
		p.close()
	}
	return nil
}

// Close is a synonym for OnShutdown().
func (e *Edge) Close() { e.OnShutdown() }

// Parse the Corefile token.
func parseEdge(c *caddy.Controller) (*Edge, error) {

	// Initialize a new Edge struct.
	e := New()

	// Declare protocols outside loop scope.
	var protocols map[int]int

	// Read in the plugin arguments.
	i := 0
	for c.Next() {

		// Make sure the plugin is only specified once in the Corefile.
		if i > 0 {
			return nil, plugin.ErrOnce
		}
		i++

		// Parse my IP address and assert that it's valid.
		var ip string
		if !c.Args(&ip) {
			return e, c.ArgErr()
		}
		e.ip = net.ParseIP(ip)
		if e.ip == nil {
			return nil, errInvalidIP
		}

		// Parse the edge cluster's longitude and latitude values.
		var lon, lat string
		if !c.Args(&lon) {
			return e, c.ArgErr()
		}
		parsedLon, err := strconv.ParseFloat(lon, 64)
		if err != nil {
			return e, err
		}
		if !c.Args(&lat) {
			return e, c.ArgErr()
		}
		parsedLat, err := strconv.ParseFloat(lat, 64)
		if err != nil {
			return e, err
		}
		e.geoCoords = NewPoint(parsedLon, parsedLat)

		// Parse and normalize the base domain.
		if !c.Args(&e.baseDomain) {
			return e, c.ArgErr()
		}
		e.baseDomain = plugin.Host(e.baseDomain).Normalize()

		// Parse the upstream addresses as the remaining args.
		// NOTE: We don't complain if there are no upstreams.
		upstreams := c.RemainingArgs()

		// A bit fiddly, but first check if we've got protocols and if so add them back in when we create the proxies.
		protocols = make(map[int]int)
		for i := range upstreams {
			protocols[i], upstreams[i] = protocol(upstreams[i])
		}

		// If parseHostPortOrFile expands a file with a lot of nameserver our accounting in protocols doesn't make
		// any sense anymore... For now: we don't care.
		upstreamHosts, err := dnsutil.ParseHostPortOrFile(upstreams...)
		if err != nil {
			return e, err
		}

		// Configure the proxies based on the list of upstream hosts.
		for i, h := range upstreamHosts {

			// Double check the port, if e.g. is 53 and the transport is TLS make it 853.
			// This can be somewhat annoying because you *can't* have TLS on port 53 then.
			switch protocols[i] {
			case TLS:
				h1, p, err := net.SplitHostPort(h)
				if err != nil {
					break
				}

				// This is more of a bug in // dnsutil.ParseHostPortOrFile that defaults to
				// 53 because it doesn't know about the tls:// // and friends (that should be fixed). Hence
				// Fix the port number here, back to what the user intended.
				if p == "53" {
					h = net.JoinHostPort(h1, "853")
				}
			}

			// We can't set tlsConfig here, because we haven't parsed it yet.
			// We set it below at the end of parseBlock, use nil now.
			p := NewProxy(h, nil /* no TLS */)
			e.proxies = append(e.proxies, p)
		}

		// Parse the extra configuration.
		for c.NextBlock() {
			if err := parseBlock(c, e); err != nil {
				return e, err
			}
		}
	}

	if e.tlsServerName != "" {
		e.tlsConfig.ServerName = e.tlsServerName
	}
	for i := range e.proxies {
		// Only set this for proxies that need it.
		if protocols[i] == TLS {
			e.proxies[i].SetTLSConfig(e.tlsConfig)
		}
		e.proxies[i].SetExpire(e.expire)
	}
	return e, nil
}

// Parses the extra plugin configuration flags in the block section of the
// plugin arguments.
func parseBlock(c *caddy.Controller, e *Edge) error {

	// See README for explanation of these arguments.
	switch c.Val() {
	case "except":
		ignore := c.RemainingArgs()
		if len(ignore) == 0 {
			return c.ArgErr()
		}
		for i := 0; i < len(ignore); i++ {
			ignore[i] = plugin.Host(ignore[i]).Normalize()
		}
		e.ignored = ignore
	case "max_fails":
		if !c.NextArg() {
			return c.ArgErr()
		}
		n, err := strconv.Atoi(c.Val())
		if err != nil {
			return err
		}
		if n < 0 {
			return fmt.Errorf("max_fails can't be negative: %d", n)
		}
		e.maxUpstreamFails = uint32(n)
	case "health_check":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		if dur < 0 {
			return fmt.Errorf("health_check can't be negative: %d", dur)
		}
		e.healthCheckInterval = dur
	case "force_tcp":
		if c.NextArg() {
			return c.ArgErr()
		}
		e.forceTCP = true
	case "dns_debug":
		if c.NextArg() {
			return c.ArgErr()
		}
		dnsDebugMode = true
	case "service_debug":
		if c.NextArg() {
			return c.ArgErr()
		}
		svcDebugMode = true
	case "tls":
		args := c.RemainingArgs()
		if len(args) > 3 {
			return c.ArgErr()
		}
		tlsConfig, err := pkgtls.NewTLSConfigFromArgs(args...)
		if err != nil {
			return err
		}
		e.tlsConfig = tlsConfig
	case "tls_servername":
		if !c.NextArg() {
			return c.ArgErr()
		}
		e.tlsServerName = c.Val()
	case "expire":
		if !c.NextArg() {
			return c.ArgErr()
		}
		dur, err := time.ParseDuration(c.Val())
		if err != nil {
			return err
		}
		if dur < 0 {
			return fmt.Errorf("expire can't be negative: %s", dur)
		}
		e.expire = dur
	case "policy":
		if !c.NextArg() {
			return c.ArgErr()
		}
		switch x := c.Val(); x {
		case "random":
			e.policy = &random{}
		case "round_robin":
			e.policy = &roundRobin{}
		default:
			return c.Errf("unknown policy '%s'", x)
		}
	default:
		return c.Errf("unknown property '%s'", c.Val())
	}

	return nil
}
