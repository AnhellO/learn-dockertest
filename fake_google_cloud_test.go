package demo_test

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type HostFixRoundTripper struct {
	Proxy http.RoundTripper
}

func (l HostFixRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	request.Host = "gcs:4443"
	res, err := l.Proxy.RoundTrip(request)
	if res != nil {
		location := res.Header.Get("Location")
		if len(location) != 0 {
			res.Header.Set("Location", strings.Replace(location, "gcs", "localhost", 1))
		}
	}
	return res, err
}

func setupGcloud() {
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "path/to/your/credentials")
}

func TestFakeGCloudStorage(t *testing.T) {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working directory: %s", err)
	}

	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "fsouza/fake-gcs-server",
		Tag:          "latest",
		Name:         "fake-gcs-server",
		ExposedPorts: []string{"4443"},
		Cmd:          []string{"-backend", "memory", "-scheme", "http", "-port", "4443", "-public-host", "gcs:4443", "-external-url", "http://gcs:4443"},
	}, func(config *docker.HostConfig) {
		// set AutoRemove to true so that stopped container goes away by itself
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{
			Name: "no",
		}
		config.Mounts = []docker.HostMount{
			{
				Target: "/data",
				Source: fmt.Sprintf("%s/examples/data", pwd),
				Type:   "bind",
			},
		}
	})
	if err != nil {
		t.Fatalf("Could not start resource: %s", err)
	}

	endpoint := fmt.Sprintf("http://localhost:%s/storage/v1/", resource.GetPort("4443/tcp"))
	t.Logf("client endpoint: %+v", endpoint)
	client, err := storage.NewClient(
		context.TODO(),
		option.WithEndpoint(endpoint),
		option.WithoutAuthentication(),
		option.WithHTTPClient(&http.Client{
			Transport: &HostFixRoundTripper{&http.Transport{}},
		}),
	)
	if err != nil {
		t.Fatalf("Could not connect to docker - failed to create client: %v", err)
	}

	const (
		bucketName  = "sample-bucket"
		fileName    = "some_file.txt"
		newFileName = "new_file.txt"
	)

	buckets, err := list(client, bucketName)
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	t.Logf("buckets: %+v\n", buckets)

	data, err := readFile(client, bucketName, fileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("contents of %s/%s: %s\n", bucketName, fileName, data)

	err = createFile(client, bucketName, newFileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("new file '%+v' created\n", newFileName)

	err = deleteFile(client, bucketName, newFileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("file %s deleted\n", newFileName)

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

func createFile(client *storage.Client, bucketName, fileName string) error {
	wc := client.Bucket(bucketName).Object(fileName).NewWriter(context.TODO())
	wc.ContentType = "text/plain"
	wc.Metadata = map[string]string{
		"x-goog-meta-foo": "foo",
		"x-goog-meta-bar": "bar",
	}

	if _, err := wc.Write([]byte("abcde\n")); err != nil {
		return fmt.Errorf("unable to write data to bucket %q, file %q: %v", bucketName, fileName, err)
	}

	if _, err := wc.Write([]byte(strings.Repeat("f", 1024*4) + "\n")); err != nil {
		return fmt.Errorf("unable to write data to bucket %q, file %q: %v", bucketName, fileName, err)
	}

	if err := wc.Close(); err != nil {
		return fmt.Errorf("unable to close bucket %q, file %q: %v", bucketName, fileName, err)
	}

	return nil
}

func readFile(client *storage.Client, bucketName, fileName string) ([]byte, error) {
	reader, err := client.Bucket(bucketName).Object(fileName).NewReader(context.TODO())
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}

func deleteFile(client *storage.Client, bucketName, fileName string) error {
	return client.Bucket(bucketName).Object(fileName).Delete(context.TODO())
}
