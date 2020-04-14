package utils

import (
	"testing"
)

func Test_IsRemoteFile(t *testing.T) {
	type testCase struct {
		filePath string
		isRemote bool
	}

	testCases := []testCase{
		{
			filePath: "http://github.com",
			isRemote: true,
		},
		{
			filePath: "../abc.txt",
			isRemote: false,
		},
		{
			filePath: "/ab/c.txt",
			isRemote: false,
		},
		{
			filePath: "abc.txt",
			isRemote: false,
		},
	}

	for _, test := range testCases {
		isRemote, err := IsRemoteFile(test.filePath)
		if err != nil {
			t.Errorf("Error checking IsRemoteFile: %v", err)
		}
		if isRemote != test.isRemote {
			t.Errorf("check if path %v is remote; expect %v, got %v", test.filePath, test.isRemote, isRemote)
		}
	}
}

func TestSplitYAML(t *testing.T) {
	tests := []struct {
		name     string
		yaml     []byte
		expected [][]byte
	}{
		{
			name:     "simple",
			yaml:     []byte("a: b\n---\nc: d"),
			expected: [][]byte{[]byte("a: b\n"), []byte("c: d\n")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resources, err := SplitYAML(test.yaml)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			for idx := range resources {
				if string(resources[idx]) != string(test.expected[idx]) {
					t.Fatalf("Resource in place %v. Got '%s', Want '%s'.", idx, resources[idx], test.expected[idx])
				}
			}
		})
	}
}
