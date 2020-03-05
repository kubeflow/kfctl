package aws

import (
	"github.com/golangplus/testing/assert"
	"testing"
)

func TestIsEksctlVersionLessThan(t *testing.T) {
	v1 := "0.1.30"
	result, _ := isEksctlVersionLessThan(v1, MINIMUM_EKSCTL_VERSION)
	assert.True(t, "eksctl version", result)

	v1 = "0.1.32"
	result, _ = isEksctlVersionLessThan(v1, MINIMUM_EKSCTL_VERSION)
	assert.False(t, "eksctl version", result)

	v1 = "0.1.33"
	result, _ = isEksctlVersionLessThan(v1, MINIMUM_EKSCTL_VERSION)
	assert.False(t, "eksctl version", result)
}
