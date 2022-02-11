package mongo_test

import (
	"context"
	"fmt"
	"log"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var dbClient *mongo.Client

func TestMongo(t *testing.T) {
	network, err := pool.Client.CreateNetwork(docker.CreateNetworkOptions{Name: "mongo_network"})
	if err != nil {
		log.Fatalf("could not create a network for mongo: %s", err)
	}

	// First start the main mongodb container
	mongodbResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository:   "mongo",
		Tag:          "latest",
		NetworkID:    network.ID,
		Name:         "mongodb",
		ExposedPorts: []string{"27017"},
		Env: []string{
			"MONGO_INITDB_ROOT_USERNAME=mongoadmin",
			"MONGO_INITDB_ROOT_PASSWORD=secret",
			"MONGO_INITDB_DATABASE=test_db",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.NeverRestart()
	})
	if err != nil {
		t.Fatalf("Could not start mongodb resource: %s", err)
	}

	// Exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err = pool.Retry(func() error {
		var err error
		dbClient, err = mongo.Connect(
			context.TODO(),
			options.Client().ApplyURI(
				fmt.Sprintf("mongodb://mongoadmin:secret@localhost:%s/test_db?authSource=admin&readPreference=primary&directConnection=true&ssl=false", mongodbResource.GetPort("27017/tcp")),
			),
		)
		if err != nil {
			return err
		}
		return dbClient.Ping(context.TODO(), nil)
	}); err != nil {
		t.Fatalf("Could not connect to docker: %s", err)
	}

	// Finally build & start the mongoseeder container
	mongoSeederResource, err := pool.BuildAndRunWithOptions("./seeder/Dockerfile", &dockertest.RunOptions{
		NetworkID: network.ID,
		Name:      "mongoseeder",
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.NeverRestart()
	})
	if err != nil {
		t.Fatalf("Could not start mongoseeder resource: %s", err)
	}

	// When you're done, kill and remove the seeder first
	if err = pool.Purge(mongoSeederResource); err != nil {
		t.Fatalf("Could not purge mongoseeder resource: %s", err)
	}

	// Then remove the main mongodb container
	if err = pool.Purge(mongodbResource); err != nil {
		t.Fatalf("Could not purge mongodb resource: %s", err)
	}

	// Disconnect mongodb client
	if err = dbClient.Disconnect(context.TODO()); err != nil {
		panic(err)
	}

	// Finally remove the network
	if err = pool.Client.RemoveNetwork(network.ID); err != nil {
		log.Fatalf("could not remove %s network: %s", network.Name, err)
	}
}
