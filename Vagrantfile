# -*k mode: ruby -*-
# vi: set ft=ruby :

$coredns_path = ENV["COREDNS_PATH"] || "coredns"
$coredns_branch = ENV["COREDNS_BRANCH"] || "master"

$num_clusters = 4
if ENV["NUM_CLUSTERS"] && ENV["NUM_CLUSTERS"].to_i > 0
    $num_clusters = ENV["NUM_CLUSTERS"].to_i
end

$box = ENV["VM_NAME"] || "optikon/demo-base"
$box_version = ENV["VM_VERSION"] || "1.0"

$central_cluster_coords = (ENV["CENTRAL_CLUSTER_COORDS"] || "55.692770,12.598624").split(/\s*,\s*/)
$edge_cluster_coords = (ENV["EDGE_CLUSTER_COORDS"] || "55.664023,12.610126,55.680770,12.543006,55.6748923,12.5534").split(/\s*,\s*/)

$extra_configs = "{\n            debug_mode\n            service_extension .optikon\n        }"

def provision_vm(config, vm_name, i)
    config.vm.hostname = vm_name
    config.vm.synced_folder ".", "/vagrant", disabled: true
    config.vm.provision "file", source: $coredns_path, destination: "/home/vagrant/.coredns"
    config.vm.box = $box
    config.vm.box_version = $box_version
    ip = "172.16.7.#{i+100}"
    config.vm.network :private_network, ip: ip
    config.vm.provision :shell, inline: "ifup eth1"
    config.vm.provision "shell", path: "scripts/reset-kube-config.sh", env: {"MYIP" => ip}, privileged: true
    config.vm.provision "file", source: "scripts/tiller.yaml", destination: "/home/vagrant/tiller.yaml"
    config.vm.provision "shell", path: "scripts/deploy-helm.sh",  privileged: true
    config.vm.provision :shell, inline: "kubectl -n kube-system replace -f /home/vagrant/.coredns/manifests/kube-dns-svc.yaml"
    config.vm.provision :shell, inline: "kubectl -n kube-system replace -f /home/vagrant/.coredns/manifests/kube-dns-depl.yaml --force"
end

Vagrant.configure("2") do |config|

    # Increase memory for Virtualbox
    config.vm.provider "virtualbox" do |vb|
          vb.memory = "4000"
          vb.cpus = 2
    end

    if $edge_cluster_coords.length != (2 * ($num_clusters-1))
        raise Vagrant::Errors::VagrantError.new, "Incorrect number of edge cluster coordinates."
    end

    # Fetches CoreDNS remote repo and moved to correct branch.
    if !File.directory?($coredns_path)
        system("git clone https://github.com/optikon/coredns " + $coredns_path)
        system("cd " + $coredns_path + " && git checkout " + $coredns_branch)
    end

    (1..$num_clusters).each do |i|
        if i == 1 #do central FIRST
            config.vm.define vm_name = "central", primary: true do |config|
                provision_vm(config, vm_name, i)
                config.vm.provision "file", source: "scripts/central.html", destination: "/home/vagrant/html/index.html"
                config.vm.provision "file", source: "scripts/optikon-api.yaml", destination: "/home/vagrant/optikon-api.yaml"
                config.vm.provision "file", source: "scripts/optikon-ui.yaml", destination: "/home/vagrant/optikon-ui.yaml"
                config.vm.provision "file", source: "scripts/pv.yaml", destination: "/home/vagrant/pv.yaml"

                config.vm.provision "shell", path: "scripts/deploy-registry.sh", privileged: true
                config.vm.provision "shell", path: "scripts/deploy-optikon.sh", privileged: true

                config.vm.provision :shell,
                    path: "scripts/replace-env-vars.sh",
                    env: {
                        "MY_IP" => "172.16.7.#{i+100}",
                        "LON" => $central_cluster_coords[0],
                        "LAT" => $central_cluster_coords[1],
                        "EXTRA_CONFIGS" => $extra_configs
                    }
                config.vm.provision :shell, inline: "kubectl -n kube-system replace -f /home/vagrant/.coredns/manifests/corefile.yaml"
                config.vm.provision :shell, path: "scripts/trigger-coredns-reload.sh"
                config.vm.provision "shell", path: "scripts/resolve.sh", privileged: true
            end
        else
          # EDGE VM CLUSTERS
            config.vm.define vm_name = "%s-%01d" % ["edge", i-1] do |config|
                provision_vm(config, vm_name, i)
                config.vm.provision "file", source: "scripts/edge-#{i-1}.html", destination: "/home/vagrant/html/index.html"
                config.vm.provision "file", source: "scripts/inject-kubeconfig.py", destination: "/home/vagrant/inject-kubeconfig.py"
                config.vm.provision "file", source: "scripts/edge-#{i-1}.json", destination: "/home/vagrant/edge-#{i-1}.json"
                config.vm.provision "shell", path: "scripts/post-to-optikon.sh", :args => "/home/vagrant/edge-#{i-1}.json"
                config.vm.provision :shell,
                    path: "scripts/replace-env-vars.sh",
                    env: {
                        "MY_IP" => "172.16.7.#{i+100}",
                        "UPSTREAMS" => "172.16.7.101:53",
                        "LON" => $edge_cluster_coords[2*(i-2)],
                        "LAT" => $edge_cluster_coords[2*(i-2)+1],
                        "EXTRA_CONFIGS" => $extra_configs
                    }
                config.vm.provision :shell, inline: "kubectl -n kube-system replace -f /home/vagrant/.coredns/manifests/corefile.yaml"
                config.vm.provision :shell, path: "scripts/trigger-coredns-reload.sh"
            end
        end
    end
end
