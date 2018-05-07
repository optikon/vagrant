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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/health.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"sync/atomic"

	"github.com/miekg/dns"
)

// For HC we send to . IN NS +norec message to the upstream. Dial timeouts and empty
// replies are considered fails, basically anything else constitutes a healthy upstream.

// Check is used as the up.Func in the up.Probe.
func (p *Proxy) Check() error {
	err := p.sendHealthCheck()
	if err != nil {
		atomic.AddUint32(&p.fails, 1)
		return err
	}
	atomic.StoreUint32(&p.fails, 0)
	return nil
}

// Sends a healthcheck ping to the proxy.
func (p *Proxy) sendHealthCheck() error {
	hcping := new(dns.Msg)
	hcping.SetQuestion(".", dns.TypeNS)
	m, _, err := p.client.Exchange(hcping, p.addr)
	if err != nil && m != nil {
		if m.Response || m.Opcode == dns.OpcodeQuery {
			err = nil
		}
	}
	return err
}
