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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/lookup.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Forward forwards the request in state as-is. Unlike Lookup that adds EDNS0 suffix to the message.
func (e *Edge) Forward(state request.Request) (*dns.Msg, error) {
	if e == nil {
		return nil, errNoEdge
	}

	fails := 0
	var upstreamErr error
	for _, proxy := range e.list() {
		if proxy.Down(e.maxUpstreamFails) {
			fails++
			if fails < len(e.proxies) {
				continue
			}
			// All upstream proxies are dead, assume healtcheck is complete broken and randomly
			// select an upstream to connect to.
			proxy = e.list()[0]
		}

		// Make the connection and receive the response.
		ret, err := proxy.connect(context.Background(), state, e.forceTCP, true)

		ret, err = truncated(ret, err)
		upstreamErr = err

		if err != nil {
			if fails < len(e.proxies) {
				continue
			}
			break
		}

		// Check if the reply is correct; if not return FormErr.
		if !state.Match(ret) {
			return state.ErrorMessage(dns.RcodeFormatError), nil
		}

		return ret, err
	}

	if upstreamErr != nil {
		return nil, upstreamErr
	}

	return nil, errNoHealthy
}

// Lookup will use name and type to forge a new message and will send that upstream. It will
// set any EDNS0 options correctly so that downstream will be able to process the reply.
// Lookup may be called with a nil f, an error is returned in that case.
func (e *Edge) Lookup(state request.Request, name string, typ uint16) (*dns.Msg, error) {
	if e == nil {
		return nil, errNoEdge
	}

	req := new(dns.Msg)
	req.SetQuestion(name, typ)
	state.SizeAndDo(req)

	state2 := request.Request{W: state.W, Req: req}

	return e.Forward(state2)
}
