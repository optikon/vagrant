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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/persistent.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"crypto/tls"
	"net"
	"time"

	"github.com/miekg/dns"
)

// persistConn holds the dns.Conn and the last used time.
type persistConn struct {
	c    *dns.Conn
	used time.Time
}

// connErr is used to communicate the connection manager.
type connErr struct {
	c   *dns.Conn
	err error
}

// transport holds the persistent cache.
type transport struct {
	conns     map[string][]*persistConn //  Buckets for udp, tcp and tcp-tls.
	expire    time.Duration             //  After this duration a connection is expired.
	addr      string
	tlsConfig *tls.Config

	dial  chan string
	yield chan connErr
	ret   chan connErr

	// Aid in testing, gets length of cache in data-race safe manner.
	lenc    chan bool
	lencOut chan int

	stop chan bool
}

// Initializes a new transport channel and manages it in a goroutine.
func newTransport(addr string, tlsConfig *tls.Config) *transport {
	t := &transport{
		conns:   make(map[string][]*persistConn),
		expire:  defaultExpire,
		addr:    addr,
		dial:    make(chan string),
		yield:   make(chan connErr),
		ret:     make(chan connErr),
		stop:    make(chan bool),
		lenc:    make(chan bool),
		lencOut: make(chan int),
	}
	go t.connManager()
	return t
}

// len returns the number of connections, used for metrics. Can only be safely
// used inside connManager() because of data races.
func (t *transport) len() int {
	l := 0
	for _, conns := range t.conns {
		l += len(conns)
	}
	return l
}

// Len returns the number of connections in the cache.
func (t *transport) Len() int {
	t.lenc <- true
	l := <-t.lencOut
	return l
}

// connManagers manages the persistent connection cache for UDP and TCP.
func (t *transport) connManager() {

Wait:
	for {
		select {
		case proto := <-t.dial:
			// Yes O(n), shouldn't put millions in here. We walk all connection until we find the first
			// one that is usable.
			var i int
			for i = 0; i < len(t.conns[proto]); i++ {
				pc := t.conns[proto][i]
				if time.Since(pc.used) < t.expire {
					// Found one, remove from pool and return this conn.
					t.conns[proto] = t.conns[proto][i+1:]
					t.ret <- connErr{pc.c, nil}
					continue Wait
				}
				// This conn has expired. Close it.
				pc.c.Close()
			}

			// Not conns were found. Connect to the upstream to create one.
			t.conns[proto] = t.conns[proto][i:]

			go func() {
				if proto != tcpTLS {
					c, err := dns.DialTimeout(proto, t.addr, dialTimeout)
					t.ret <- connErr{c, err}
					return
				}
				c, err := dns.DialTimeoutWithTLS("tcp", t.addr, t.tlsConfig, dialTimeout)
				t.ret <- connErr{c, err}
			}()

		case conn := <-t.yield:

			// no proto here, infer from config and conn
			if _, ok := conn.c.Conn.(*net.UDPConn); ok {
				t.conns["udp"] = append(t.conns["udp"], &persistConn{conn.c, time.Now()})
				continue Wait
			}

			if t.tlsConfig == nil {
				t.conns["tcp"] = append(t.conns["tcp"], &persistConn{conn.c, time.Now()})
				continue Wait
			}

			t.conns[tcpTLS] = append(t.conns[tcpTLS], &persistConn{conn.c, time.Now()})

		case <-t.stop:
			return

		case <-t.lenc:
			l := 0
			for _, conns := range t.conns {
				l += len(conns)
			}
			t.lencOut <- l
		}
	}
}

// Dial dials the address configured in transport, potentially reusing a connection or creating a new one.
func (t *transport) Dial(proto string) (*dns.Conn, error) {

	// If tls has been configured, use it.
	if t.tlsConfig != nil {
		proto = tcpTLS
	}

	t.dial <- proto
	c := <-t.ret

	return c.c, c.err
}

// Yield returns the connection to transport for reuse.
func (t *transport) Yield(c *dns.Conn) {
	t.yield <- connErr{c, nil}
}

// Stop stops the transport's connection manager.
func (t *transport) Stop() { t.stop <- true }

// SetExpire sets the connection expire time in transport.
func (t *transport) SetExpire(expire time.Duration) { t.expire = expire }

// SetTLSConfig sets the TLS config in transport.
func (t *transport) SetTLSConfig(cfg *tls.Config) { t.tlsConfig = cfg }
