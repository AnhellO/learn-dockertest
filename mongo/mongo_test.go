package mongo_test

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Restaurant struct {
	ID      primitive.ObjectID `json:"id" bson:"_id"`
	Address struct {
		Building string    `json:"building" bson:"building"`
		Coord    []float64 `json:"coord" bson:"coord"`
		Street   string    `json:"street" bson:"street"`
		Zipcode  string    `json:"zipcode" bson:"zipcode"`
	} `json:"address" bson:"address"`
	Borough string `json:"borough" bson:"borough"`
	Cuisine string `json:"cuisine" bson:"cuisine"`
	Grades  []struct {
		Date  time.Time `json:"date" bson:"date"`
		Grade string    `json:"grade" bson:"grade"`
		Score int       `json:"score" bson:"score"`
	} `json:"grades" bson:"grades"`
	Name         string `json:"name" bson:"name"`
	RestaurantID string `json:"restaurant_id" bson:"restaurant_id"`
}

var dbClient *mongo.Client

func TestMongo(t *testing.T) {
	time.Local = time.UTC
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
		PortBindings: map[docker.Port][]docker.PortBinding{
			"27017/tcp": {
				{
					HostIP:   "localhost",
					HostPort: "27017/tcp",
				},
			},
		},
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
		// config.AutoRemove = true
		config.RestartPolicy = docker.RestartOnFailure(5)
	})
	if err != nil {
		t.Fatalf("Could not start mongoseeder resource: %s", err)
	}

	// Wait a bit so data gets populated
	time.Sleep(5 * time.Second)

	// Now actually test the data in mongodb by executing some queries
	clientOptions := options.Client().ApplyURI(fmt.Sprintf("mongodb://mongoadmin:secret@localhost:%s/test_db?authSource=admin&readPreference=primary&directConnection=true&ssl=false", mongodbResource.GetPort("27017/tcp")))

	ctx := context.TODO()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		t.Fatalf("Failed to connect to mongodb resource: %s", err)
	}

	// Check connection
	err = client.Ping(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to ping mongodb resource: %s", err)
	}

	// Execute the actual queries
	t.Log("Querying documents...")
	document, err := GetRestaurantByRestaurantId("40356649", client)
	if err != nil {
		t.Errorf("Could not find document '40356649': %s", err)
	}

	t.Logf("Resulting document: %+v", document)
	if document.Name != "Regina Caterers" {
		t.Errorf("The restaurant is not the one we are looking for -> ID: %s - Name: %s", document.RestaurantID, document.Name)
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

// GetRestaurantByRestaurantId tries to look in the DB for the restaurant document with the specified restaurantId
func GetRestaurantByRestaurantId(restaurantId string, client *mongo.Client) (*Restaurant, error) {
	com := client.Database("test_db").Collection("restaurants")
	ctx := context.TODO()

	var restaurant Restaurant
	err := com.FindOne(ctx, bson.M{"restaurant_id": restaurantId}).Decode(&restaurant)

	if err != nil {
		return nil, err
	}

	return &restaurant, nil
}
