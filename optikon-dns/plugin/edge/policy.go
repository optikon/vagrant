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
 * `forward` plugin (https://github.com/coredns/coredns/blob/master/plugin/forward/policy.go),
 * including various modifications by Cisco Systems, Inc.
 */

package edge

import (
	"math/rand"
	"sync/atomic"
)

// policyType tells the plugin what policy for selecting upstream it uses.
type policyType int

const (
	randomPolicy policyType = iota
	roundRobinPolicy
)

// Policy defines a policy we use for selecting upstreams.
type Policy interface {
	List([]*Proxy) []*Proxy
	String() string
}

// The policy that implements random upstream selection.
type random struct{}

// String returns the string representation of the random policy.
func (r *random) String() string { return "random" }

// List returns the given proxies in an order following the random policy.
func (r *random) List(p []*Proxy) []*Proxy {
	switch len(p) {
	case 1:
		return p
	case 2:
		if rand.Int()%2 == 0 {
			return []*Proxy{p[1], p[0]} // swap
		}
		return p
	}
	perms := rand.Perm(len(p))
	rnd := make([]*Proxy, len(p))
	for i, p1 := range perms {
		rnd[i] = p[p1]
	}
	return rnd
}

// The policy that selects hosts based on round robin ordering.
type roundRobin struct {
	robin uint32
}

// String returns the string representation of the roundRobin policy.
func (r *roundRobin) String() string { return "round_robin" }

// List returns the given proxies in an order following the round robin policy.
func (r *roundRobin) List(p []*Proxy) []*Proxy {
	poolLen := uint32(len(p))
	i := atomic.AddUint32(&r.robin, 1) % poolLen
	robin := []*Proxy{p[i]}
	robin = append(robin, p[:i]...)
	robin = append(robin, p[i+1:]...)
	return robin
}
