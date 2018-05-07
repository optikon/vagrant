# edge

## Name

*edge* - redirects clients to the closest edge cluster running the Kubernetes service that they request

## Description

This plugin is responsible for resolving incoming client requests by either returning its own IP if the service is already running on this cluster, otherwise it performs a lookup in its local `serviceDNS->[]edgeSite` mapping and tried to find an edge site that is running the requested service closest to the requested. If no such service can be found, it forwards the request up to its (possibly many) upstream proxies, which perform the same process in a CDN-like behavior. Whatever gets returned from upstream is used as the authoritative answer and is sent back to the client.

This plugin also runs a routine daemon process that calls the Kubernetes cluster API to watch its running services, and pushes that any service updates up to its upstream proxies so they can update their local tables and accurately resolve future requests. This plugin also plays the roll of an upstream proxy, listening for service events to be pushed up from downstream edge sites via a simple RESTful API passing JSON data.

## Syntax

~~~ txt
edge MY_IP LONGITUDE LATITUDE BASE_DOMAIN UPSTREAMS...
~~~

* __MY_IP__ is the address of the DNS server running this plugin.
* __LONGITUDE__ is the longitude coordinate of the DNS server running this plugin.
* __LATITUDE__ is the latitude coordinate of the DNS server running this plugin.
* __BASE_DOMAIN__ is the base domain to match against incoming DNS requests.
* __UPSTREAMS...__ are the upstream proxies used to resolve requests that can't be resolved locally. The __UPSTREAMS__ syntax allows you to specify a protocol, `tls://9.9.9.9` or `dns://` (or no protocol) for plain DNS. The number of upstreams is limited to 15.

Multiple upstreams are randomized (see `policy`) on first use. When a healthy proxy returns an error during the exchange the next upstream in the list is tried.

Extra configuration is available through the expanded syntax:

~~~ txt
edge MY_IP LONGITUDE LATITUDE BASE_DOMAIN UPSTREAMS... {
    except IGNORED_NAMES...
    force_tcp
    expire DURATION
    max_fails INTEGER
    tls CERT KEY CA
    tls_servername NAME
    policy random|round_robin|sequential
    health_check DURATION
    dns_debug
    service_debug
}
~~~

* __MY_IP__, __LONGITUDE__, __LATITUDE__, __SVC_READ_INTERVAL__, __SVC_PUSH_INTERVAL__, __BASE_DOMAIN__, and __UPSTREAMS...__ as above.
* __IGNORED_NAMES__ in `except` is a space-separated list of domains to exclude from DNS resolution. Requests that match none of these names will be passed through.
* `force_tcp`, use TCP even when the request comes in over UDP.
* `max_fails` is the number of subsequent failed health checks that are needed before considering an upstream to be down. If 0, the upstream will never be marked as down (nor health checked). Default is 2.
* `expire` __DURATION__, expire (cached) connections after this time, the default is 10s.
* `tls` __CERT__ __KEY__ __CA__ define the TLS properties for TLS connection. From 0 to 3 arguments can be provided with the meaning as described below
  * `tls` - no client authentication is used, and the system CAs are used to verify the server certificate
  * `tls` __CA__ - no client authentication is used, and the file CA is used to verify the server certificate
  * `tls` __CERT__ __KEY__ - client authentication is used with the specified cert/key pair.
    The server certificate is verified with the system CAs
  * `tls` __CERT__ __KEY__  __CA__ - client authentication is used with the specified cert/key pair.
    The server certificate is verified using the specified CA file
* `tls_servername` __NAME__ allows you to set a server name in the TLS configuration; for instance 9.9.9.9 needs this to be set to `dns.quad9.net`.
* `policy` specifies the policy to use for selecting upstream servers. The default is `random`.
* `health_check`, use a different __DURATION__ for health checking, the default duration is 0.5s.
* `dns_debug`, turn on debug-level logging for DNS-related logic.
* `service_debug`, turn on debug-level logging for service-related logic.

Also note the TLS config is "global" for the whole upstream proxy if you need a different `tls-name` for different upstreams you're out of luck.

## Examples

An example Corefile might look like

~~~ corefile
.:53 {
    errors
    health
    log
    cache 30
    kubernetes cluster.local {
       fallthrough
    }
    edge 172.16.7.102 43.264 36.694 . 172.16.7.101:53 172.16.7.105:53 {
        dns_debug
        service_debug
    }
    proxy . 8.8.8.8:53
}
~~~
