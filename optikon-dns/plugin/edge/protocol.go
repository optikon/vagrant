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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/protocol.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

// Copied from coredns/core/dnsserver/address.go

import (
	"strings"
)

const (
	_dns = "dns"
	_tls = "tls"
)

// Supported protocols.
const (
	DNS = iota + 1
	TLS
)

// protocol returns the protocol of the string s. The second string returns s
// with the prefix chopped off.
func protocol(s string) (int, string) {
	switch {
	case strings.HasPrefix(s, _tls+"://"):
		return TLS, s[len(_tls)+3:]
	case strings.HasPrefix(s, _dns+"://"):
		return DNS, s[len(_dns)+3:]
	}
	return DNS, s
}
