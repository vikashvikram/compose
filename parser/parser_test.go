package parser

import (
	"os"
	"reflect"
	"testing"
)

func TestParseFile(t *testing.T) {
	content := `
version: "3.8"
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    environment:
      - DEBUG=true
      - PING
    depends_on:
      - db
  db:
    image: postgres:15
    environment:
      POSTGRES_DB: mydb
      POSTGRES_USER: user
    volumes:
      - db-data:/var/lib/postgresql/data
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 512M
    build: ./db-dir
`
	tmpfile, err := os.CreateTemp("", "docker-compose-test-*.yml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	os.Setenv("PING", "pong")
	defer os.Unsetenv("PING")

	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Fatal(err)
	}

	config, err := ParseFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if config.Version != "3.8" {
		t.Errorf("Expected version 3.8, got %s", config.Version)
	}

	web, ok := config.Services["web"]
	if !ok {
		t.Fatal("Expected web service")
	}

	if web.Image != "nginx:alpine" {
		t.Errorf("Expected nginx:alpine, got %s", web.Image)
	}

	expectedWebPorts := []string{"8080:80"}
	if !reflect.DeepEqual(web.Ports, expectedWebPorts) {
		t.Errorf("Expected ports %v, got %v", expectedWebPorts, web.Ports)
	}

	expectedWebEnv := EnvMap{"DEBUG": "true", "PING": "pong"}
	if !reflect.DeepEqual(web.Environment, expectedWebEnv) {
		t.Errorf("Expected environment %v, got %v", expectedWebEnv, web.Environment)
	}

	expectedWebDeps := DependsOnList{"db"}
	if !reflect.DeepEqual(web.DependsOn, expectedWebDeps) {
		t.Errorf("Expected depends_on %v, got %v", expectedWebDeps, web.DependsOn)
	}

	db, ok := config.Services["db"]
	if !ok {
		t.Fatal("Expected db service")
	}

	expectedDbEnv := EnvMap{"POSTGRES_DB": "mydb", "POSTGRES_USER": "user"}
	if !reflect.DeepEqual(db.Environment, expectedDbEnv) {
		t.Errorf("Expected db env %v, got %v", expectedDbEnv, db.Environment)
	}

	if db.Build == nil || db.Build.Context != "./db-dir" {
		t.Errorf("Expected build context './db-dir', got %+v", db.Build)
	}

	if db.Deploy == nil || db.Deploy.Resources.Limits.CPUs != "0.5" || db.Deploy.Resources.Limits.Memory != "512M" {
		t.Errorf("Expected resource limits cpu: 0.5, memory: 512M, got %+v", db.Deploy)
	}
}

func TestInterpolate(t *testing.T) {
	dotEnv := map[string]string{
		"PORT": "3000",
		"HOST": "localhost",
	}

	os.Setenv("VERSION", "1.0.0")
	defer os.Unsetenv("VERSION")

	tests := []struct {
		input string
		want  string
	}{
		{
			input: "image: node:$VERSION",
			want:  "image: node:1.0.0",
		},
		{
			input: "port: ${PORT:-8080}",
			want:  "port: 3000",
		},
		{
			input: "port: ${MISSING:-8080}",
			want:  "port: 8080",
		},
		{
			input: "host: $HOST",
			want:  "host: localhost",
		},
		{
			input: "escaped: $$VERSION",
			want:  "escaped: $VERSION",
		},
	}

	for _, tt := range tests {
		got := interpolate(tt.input, dotEnv)
		if got != tt.want {
			t.Errorf("interpolate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
