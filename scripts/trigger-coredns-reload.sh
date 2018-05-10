#!/bin/bash

# Patches the running coredns deployment with the current date as an integer.
# This forces the pods in the deployment to reboot and reload the custom
# Corefile we've provisioned for coredns.
kubectl -n kube-system patch deployment coredns -p "{\"spec\":{\"template\":{\"metadata\":{\"annotations\":{\"date\":\"`date +'%s'`\"}}}}}"

# Updates the CoreDNS image to be a custom image.
# kubectl -n kube-system set image deployment/coredns coredns=optikon/coredns:2.1.0
