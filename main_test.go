package demo_test

import (
	"log"
	"os"
	"testing"

	_ "github.com/jackc/pgx/stdlib"
	"github.com/ory/dockertest/v3"
)

var pool *dockertest.Pool

func TestMain(m *testing.M) {
	setupGcloud()
	var err error
	pool, err = dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
		os.Exit(1)
	}
	code := m.Run()
	os.Exit(code)
}
