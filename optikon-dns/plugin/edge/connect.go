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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/connect.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"time"

	"github.com/coredns/coredns/request"

	"github.com/miekg/dns"
	"golang.org/x/net/context"
)

// Establishes a connection and forwards a message to the upstream proxy.
func (p *Proxy) connect(ctx context.Context, state request.Request, forceTCP, metric bool) (*dns.Msg, error) {

	proto := state.Proto()
	if forceTCP {
		proto = "tcp"
	}

	conn, err := p.Dial(proto)
	if err != nil {
		return nil, err
	}

	// Set buffer size correctly for this client.
	conn.UDPSize = uint16(state.Size())
	if conn.UDPSize < 512 {
		conn.UDPSize = 512
	}

	conn.SetWriteDeadline(time.Now().Add(timeout))
	if err := conn.WriteMsg(state.Req); err != nil {
		conn.Close() // not giving it back
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(timeout))
	ret, err := conn.ReadMsg()
	if err != nil {
		conn.Close() // not giving it back
		return nil, err
	}

	p.Yield(conn)

	return ret, nil
}
