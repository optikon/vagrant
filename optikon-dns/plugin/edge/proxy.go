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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/proxy.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/coredns/coredns/plugin/pkg/up"

	"github.com/miekg/dns"
)

const (
	tcpTLS              = "tcp-tls"
	dialTimeout         = 4 * time.Second
	timeout             = 2 * time.Second
	healthCheckDuration = 500 * time.Millisecond
	pushPort            = "8053"
	pushProtocol        = "http"
)

// Proxy defines an upstream host.
type Proxy struct {
	addr   string
	client *dns.Client

	// Connection caching.
	expire    time.Duration
	transport *transport

	// Health checking.
	probe *up.Probe
	fails uint32

	// Service push connection.
	pushAddr string
	pushChan chan struct{}
}

// NewProxy returns a new proxy.
func NewProxy(addr string, tlsConfig *tls.Config) *Proxy {
	var host string
	u, err := url.Parse(addr)
	if err == nil {
		host, _, err = net.SplitHostPort(u.Host)
		if err != nil {
			log.Fatalf("could not parse upstream network address (%v)", err)
		}
	} else {
		host, _, err = net.SplitHostPort(addr)
		if err != nil {
			log.Fatalf("could not parse upstream network address (%v)", err)
		}
	}
	p := &Proxy{
		addr:      addr,
		fails:     0,
		probe:     up.New(),
		transport: newTransport(addr, tlsConfig),
		pushAddr:  newPushAddr(host),
		pushChan:  make(chan struct{}),
	}
	p.client = dnsClient(tlsConfig)
	return p
}

// dnsClient returns a client used for health checking.
func dnsClient(tlsConfig *tls.Config) *dns.Client {
	c := new(dns.Client)
	c.Net = "udp"
	// TODO(miek): this should be half of healthCheckDuration?
	c.ReadTimeout = 1 * time.Second
	c.WriteTimeout = 1 * time.Second
	if tlsConfig != nil {
		c.Net = tcpTLS
		c.TLSConfig = tlsConfig
	}
	return c
}

// SetTLSConfig sets the TLS config in the lower p.transport.
func (p *Proxy) SetTLSConfig(cfg *tls.Config) { p.transport.SetTLSConfig(cfg) }

// SetExpire sets the expire duration in the lower p.transport.
func (p *Proxy) SetExpire(expire time.Duration) { p.transport.SetExpire(expire) }

// Dial connects to the host in p with the configured transport.
func (p *Proxy) Dial(proto string) (*dns.Conn, error) { return p.transport.Dial(proto) }

// Yield returns the connection to the pool.
func (p *Proxy) Yield(c *dns.Conn) { p.transport.Yield(c) }

// Healthcheck kicks off a round of health checks for this proxy.
func (p *Proxy) Healthcheck() { p.probe.Do(p.Check) }

// Down returns true if this proxy is down, i.e. has *more* fails than maxUpstreamFails.
func (p *Proxy) Down(maxUpstreamFails uint32) bool {
	if maxUpstreamFails == 0 {
		return false
	}
	fails := atomic.LoadUint32(&p.fails)
	return fails > maxUpstreamFails
}

// Stops the health checking and service pushing goroutines.
func (p *Proxy) close() {
	close(p.pushChan)
	p.probe.Stop()
	p.transport.Stop()
}

// Starts the proxy's healthchecking.
func (p *Proxy) start(healthCheckDuration time.Duration) {
	p.probe.Start(healthCheckDuration)
}

// Creates the network address for pushing service updates.
func newPushAddr(host string) string {
	return fmt.Sprintf("%s://%s:%s", pushProtocol, host, pushPort)
}

// Pushes service events upstream.
func (p *Proxy) pushServiceEvent(meta Site, event ServiceEvent) error {
	update := ServiceTableUpdate{
		Meta:  meta,
		Event: event,
	}
	jsn, err := json.Marshal(update)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", p.pushAddr, bytes.NewBuffer(jsn))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	go func() {
		for {
			resp, err := client.Do(req)
			if err == nil {
				if resp.StatusCode != 200 {
					log.Errorf("received a not-OK response from upstream: %d", resp.StatusCode)
				}
				resp.Body.Close()
				return
			}
			log.Errorf("received error while making upstream push request: %v (trying again)", err)
			resp.Body.Close()
			time.Sleep(time.Second * 10)
		}
	}()
	return nil
}
