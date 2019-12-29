package kfconfig

import (
	"io"
	"io/ioutil"
	"os"
	"path"
	"testing"
)

func TestSyncCache(t *testing.T) {
	type testCase struct {
		input    *KfConfig
		expected []Cache
	}

	// Verify that we can sync some files.
	testDir, _ := ioutil.TempDir("", "")

	srcDir := path.Join(testDir, "src")
	err := os.Mkdir(srcDir, os.ModePerm)

	if err != nil {
		t.Fatalf("Failed to create directoy; %v", err)
	}

	ioutil.WriteFile(path.Join(srcDir, "file1"), []byte("hello world"), os.ModePerm)

	// Verify that we can unpack a local tarball and use it.
	tarballName := "c0e81bedec9a4df8acf568cc5ccacc4bc05a3b38.tar.gz"
	from, err := os.Open(path.Join("./testdata", tarballName))
	if err != nil {
		t.Fatalf("failed to open tarball file: %v", err)
	}
	tarballPath := path.Join(srcDir, tarballName)
	to, err := os.OpenFile(tarballPath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		t.Fatalf("failed to open new file location fortarball file: %v", err)
	}
	if _, err = io.Copy(to, from); err != nil {
		t.Fatalf("tarball copy is failed: %v", err)
	}

	repoName := "testRepo"

	testCases := []testCase{
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: path.Join(testDir, "app1"),
					Repos: []Repo{{
						Name: repoName,
						URI:  srcDir,
					},
					},
				},
			},
			expected: []Cache{
				{
					Name:      repoName,
					LocalPath: path.Join(testDir, "app1", ".cache", repoName),
				},
			},
		},
		{
			input: &KfConfig{
				Spec: KfConfigSpec{
					AppDir: path.Join(testDir, "app2"),
					Repos: []Repo{{
						Name: repoName,
						URI:  "file:" + tarballPath,
					},
					},
				},
			},
			expected: []Cache{
				{
					Name:      repoName,
					LocalPath: path.Join(testDir, "app2", ".cache", repoName, "kubeflow-manifests-c0e81be"),
				},
			},
		},
	}

	for _, c := range testCases {
		err = c.input.SyncCache()

		if err != nil {
			t.Fatalf("Could not sync cache; %v", err)
		}

		actual := c.input.Status.Caches[0].LocalPath
		expected := c.expected[0].LocalPath
		if actual != expected {
			t.Fatalf("LocalPath; got %v; want %v", actual, expected)
		}
	}
}
