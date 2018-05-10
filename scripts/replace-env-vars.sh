#!/bin/bash

ROOTDIR=/home/vagrant/.coredns/manifests

# Replace environmental variables in file with their respective values.
# I could not pipe in and pipe out to the same file, so I had to devise this
# ugly solution.
envsubst < $ROOTDIR/corefile.yaml > $ROOTDIR/corefile.yaml.back
rm -f $ROOTDIR/corefile.yaml
mv $ROOTDIR/corefile.yaml.back $ROOTDIR/corefile.yaml
