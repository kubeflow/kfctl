package utils

import (
	"bytes"
	"github.com/pkg/errors"
	"io/ioutil"
	"sigs.k8s.io/kustomize/kyaml/kio"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

func ReadYaml(path string) ([]*yaml.RNode, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "Error reading path %v", path)
	}

	input := bytes.NewReader(data)
	reader := kio.ByteReader{
		Reader: input,
		// We need to disable adding reader annotations because
		// we want to run some checks about whether annotations are set and
		// adding those annotations interferes with that.
		OmitReaderAnnotations: true,
	}

	nodes, err := reader.Read()

	if err != nil {
		return nil, errors.Wrapf(err, "Error unmarshaling %v", path)
	}

	return nodes, nil
}

func WriteYaml(nodes []*yaml.RNode) ([]byte, error) {
	var b bytes.Buffer
	writer := kio.ByteWriter{
		Writer: &b,
	}

	if err := writer.Write(nodes); err != nil {
		return []byte{}, err
	}

	return b.Bytes(), nil
}
