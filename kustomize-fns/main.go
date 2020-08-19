// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main implements example kpt-functions
package main

import (
	"bytes"
	"fmt"
	ip "github.com/kubeflow/kfctl/kustomize-fns/image-prefix"
	rn "github.com/kubeflow/kfctl/kustomize-fns/remove-namespace"
	vs "github.com/kubeflow/kfctl/kustomize-fns/virtual-service"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/kio/filters"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var debugPath string

func main() {
	cmd := &cobra.Command{
		Use:          "config-function",
		SilenceUsage: true, // don't print usage on an error
		RunE:         RunE,
	}

	// Add a debug command. Instead of taking resourceList as stdIn we will read it from a file.
	// The resourceList can be created using kpt e.g
	// kpt fn source ${DIR} --function-config=./${FUNCTION_CONFIG.YAML}  > /tmp/stdin.yaml
	// You can then run this in a debugger in order to debug.
	debug := &cobra.Command{
		Use:          "debug",
		SilenceUsage: true, // don't print usage on an error
		RunE:         DebugRunE,
	}

	debug.Flags().StringVarP(&debugPath, "file", "f", "", "Path to the file containing the resource list to process. This can be produced using kpt; e.g kpt fn source ${DIR} --function-config=./${FUNCTION_CONFIG.YAML}")

	cmd.AddCommand(debug)

	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred running your function: %v\n", err)
		os.Exit(1)
	}
}

// DebugRunE is the cobra entrypoint when running in debug mode.
func DebugRunE(_ *cobra.Command, _ []string) error {
	data, readErr := ioutil.ReadFile(debugPath)
	if readErr != nil {
		return errors.Wrapf(readErr, "Error reading: %v", debugPath)
	}

	input := bytes.NewReader(data)

	d := Dispatcher{}

	return d.run(input)
}

// RunE is the cobra entrypoint when taking input from stdin
func RunE(_ *cobra.Command, _ []string) error {
	d := Dispatcher{}

	return d.run(os.Stdin)
}


// Dispatcher dispatches to the matching API
type Dispatcher struct {
	// IO hanldes reading / writing Resources
	IO *kio.ByteReadWriter
}

func (d *Dispatcher) run(input io.Reader) error {
	d.IO = &kio.ByteReadWriter{
		Reader:                input,
		Writer:                os.Stdout,
		KeepReaderAnnotations: true,
	}

	return kio.Pipeline{
		Inputs: []kio.Reader{d.IO},
		Filters: []kio.Filter{
			d, // invoke the API
			&filters.MergeFilter{},
			&filters.FileSetter{FilenamePattern: filepath.Join("config", "%n.yaml")},
			&filters.FormatFilter{},
		},
		Outputs: []kio.Writer{d.IO},
	}.Execute()
}

// dispatchTable maps configFunction Kinds to implementations
var dispatchTable = map[string]func() kio.Filter{
	ip.Kind: ip.Filter,
	rn.Kind: rn.Filter,
	vs.Kind: vs.Filter,
}

func (d *Dispatcher) Filter(inputs []*yaml.RNode) ([]*yaml.RNode, error) {
	// parse the API meta to find which API is being invoked
	meta, err := d.IO.FunctionConfig.GetMeta()
	if err != nil {
		return nil, err
	}

	if meta.Kind == "" {
		fmt.Fprintf(os.Stderr, "Kind is empty; keep going: %v", meta)
		return inputs, nil
	}
	fmt.Fprintf(os.Stderr, "Dispatching for meta: %v", meta)
	fmt.Fprintf(os.Stderr, "Dispatching for kind: %v", meta.Kind)
	// find the implementation for this API
	fn := dispatchTable[meta.Kind]
	if fn == nil {
		return nil, fmt.Errorf("unsupported API type: %s", meta.Kind)
	}

	// dispatch to the implementation
	fltr := fn()

	// initializes the object from the config
	if err := yaml.Unmarshal([]byte(d.IO.FunctionConfig.MustString()), fltr); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "%s\n", d.IO.FunctionConfig.MustString())
		os.Exit(1)
	}
	return fltr.Filter(inputs)
}

// readYaml reads the specified path and returns an RNode.
func readYaml(path string) ([]*yaml.RNode, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading path %v", path)
	}

	input := bytes.NewReader(data)
	reader := kio.ByteReader{
		Reader: input,
		// We need to disable adding reader annotations because these should have already been added
		// when we ran kpt fn source to produce the file.
		OmitReaderAnnotations: true,
	}

	nodes, err := reader.Read()

	if err != nil {
		return nil, errors.Wrapf(err, "Error unmarshaling %v", path)
	}

	return nodes, nil
}
