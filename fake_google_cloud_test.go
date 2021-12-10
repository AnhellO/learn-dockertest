package demo_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func setupGcloud() {
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "path/to/your/credentials")
}

func TestFakeGCloudStorage(t *testing.T) {
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "fsouza/fake-gcs-server",
		Tag:          "latest",
		Name:         "fake-gcs-server",
		ExposedPorts: []string{"4443"},
		Cmd:          []string{"-scheme", "http"},
		Mounts:       []string{"/home/angel/Documents/go/src/github.com/AnhellO/learn-dockertest/examples/data:/data"},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
	})
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}

	client, err := storage.NewClient(
		context.TODO(),
		option.WithEndpoint(fmt.Sprintf("http://localhost:%s/storage/v1/", resource.GetPort("4443/tcp"))),
	)
	if err != nil {
		t.Fatalf("Could not connect to docker - failed to create client: %v", err)
	}

	const (
		bucketName = "sample-bucket"
		fileKey    = "some_file.txt"
	)

	buckets, err := list(client, bucketName)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	t.Logf("buckets: %+v\n", buckets)

	data, err := downloadFile(client, bucketName, fileKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("contents of %s/%s: %s\n", bucketName, fileKey, data)

	err = deleteFile(client, bucketName, fileKey)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		err := pool.Purge(resource)
		if err != nil {
			t.Logf("Could not purge resource: %s", err)
		}
	})
}

func list(client *storage.Client, bucketName string) ([]string, error) {
	var objects []string
	it := client.Bucket(bucketName).Objects(context.Background(), &storage.Query{})
	for {
		oattrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		objects = append(objects, oattrs.Name)
	}
	return objects, nil
}

func downloadFile(client *storage.Client, bucketName, fileKey string) ([]byte, error) {
	reader, err := client.Bucket(bucketName).Object(fileKey).NewReader(context.TODO())
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}

func deleteFile(client *storage.Client, bucketName, fileKey string) error {
	return client.Bucket(bucketName).Object(fileKey).Delete(context.TODO())
}
