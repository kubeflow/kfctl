#!/usr/bin/env bash

#
# this copies kubeflow/bootstrap/{go.*,cmd/kfctl,pkg} to here and updates all references 
# github.com/kubeflow/kubeflow/bootstrap -> github.com/kubeflow/kfctl

kfctldir=$(dirname $0)/../../kubeflow/bootstrap

preclean()
{
  rm $(find config pkg -name 'zz_generated*')
  rm -rf go.* cmd pkg config/{types.go,doc.go}
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
  cp -r $kfctldir/cmd/{kfctl,plugins} cmd
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
  find go.mod cmd pkg -type f -exec grep -l 'github.com\/kubeflow\/kubeflow\/bootstrap\/' {} \;
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
}

preclean && cpdirs && updatefiles
