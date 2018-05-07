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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/truncated.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import "github.com/miekg/dns"

// truncated looks at the error and if truncated return a nil errror
// and a possible reconstructed dns message if that was nil.
func truncated(ret *dns.Msg, err error) (*dns.Msg, error) {

	// If you query for instance ANY isc.org; you get a truncated query back which miekg/dns fails to unpack
	// because the RRs are not finished. The returned message can be useful or useless. Return the original
	// query with some header bits set that they should retry with TCP.
	if err != dns.ErrTruncated {
		return ret, err
	}

	// We may or may not have something sensible... if not reassemble something to send to the client.
	m := ret
	if ret == nil {
		m = new(dns.Msg)
		m.SetReply(ret)
		m.Truncated = true
		m.Authoritative = true
		m.Rcode = dns.RcodeSuccess
	}

	return m, nil
}
