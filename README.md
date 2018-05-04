# optikon-vagrant

Vagrant environment used for Kubecon EU 2018 demo

### Steps
1. `vagrant up` : this step will start 4 VMs, one central and 3 edge VMs. Each being a 1 node kubernetes cluster. It also starts optikon.
2. Configure `resolve.conf` properly to force clients (laptop, central) to DNS with the correct edge site.
3. Open `172.16.7.101:30800/` to see Optikon UI
4. Upload Helm charts
