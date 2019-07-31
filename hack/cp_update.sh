#!/usr/bin/env bash

#
# this copies kubeflow/bootstrap/{go.*,cmd/kfctl,pkg} to here and updates all references 
# github.com/kubeflow/kubeflow/bootstrap -> github.com/kubeflow/kfctl

kfctldir=$(dirname $0)/../../kubeflow/bootstrap

preclean()
{
  rm -rf go.* cmd pkg config
}

cpdirs() 
{
  if [[ ! -d $kfctldir ]]; then
    echo invalid directory $kfctldir >&2
    exit
  fi
  if [[ ! -d cmd ]]; then
    mkdir cmd
  fi
  cp -r $kfctldir/cmd/kfctl cmd
  if [[ ! -d pkg ]]; then
    mkdir pkg
  fi
  cp -r $kfctldir/pkg .
  if [[ ! -d config ]]; then
    mkdir config
  fi
  cp -r $kfctldir/config/{types.go,doc.go} config 
  cp $kfctldir/go.mod .
}

findfiles() 
{
  cd $kfctldir 
  find go.mod cmd pkg -type f -exec grep -l 'github.com\/kubeflow\/kubeflow\/bootstrap\/' {} \; | grep -v zz_generated
}

updatefiles() 
{
  for i in $(findfiles); do
    ex -s $i <<EOF
%s#github.com/kubeflow/kubeflow/bootstrap#github.com/kubeflow/kfctl#g
w
q
EOF
  done
  ex -s go.mod <<EOF1
%s#\.\./components/profile-controller#../kubeflow/components/profile-controller#
w
q
EOF1
}

preclean && cpdirs && updatefiles
