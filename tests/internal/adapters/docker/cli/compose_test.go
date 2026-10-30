// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli_test

// Verifies:
//   - compose stop, up and pull name the project, its folder and every compose file, as compose
//     recorded them; up runs in the background and pulls quietly; pull passes what is built
//   - A failing compose is an error
//   - Its version is asked short; a docker without the plugin fails
//   - An image is pulled quietly and removed by its name

import (
	"context"
	"testing"

	"github.com/nauarch/limen/internal/adapters/docker/cli"
	"github.com/nauarch/limen/internal/contracts/host"
)

func TestComposeNamesTheProjectFully(t *testing.T) {
	var ran []string
	c := cli.NewCompose(script(map[string]string{"compose": ""}, &ran))
	p := host.Project{Name: "dns", Dir: "/srv/dns", Files: []string{"/srv/dns/docker-compose.yaml", "/srv/dns/override.yaml"}}
	if err := c.Stop(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := c.Up(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if err := c.Pull(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	const named = "compose --project-name dns --project-directory /srv/dns --file /srv/dns/docker-compose.yaml --file /srv/dns/override.yaml"
	if len(ran) != 3 || ran[0] != named+" stop" || ran[1] != named+" up --detach --quiet-pull" || ran[2] != named+" pull --ignore-buildable --quiet" {
		t.Errorf("%q", ran)
	}
	if err := cli.NewCompose(script(nil, &ran)).Up(context.Background(), p); err == nil {
		t.Error("a failing compose passed")
	}
}

func TestPullAndRemoveNameTheImage(t *testing.T) {
	var ran []string
	c := cli.New(script(map[string]string{"image pull": "", "image rm": ""}, &ran))
	if err := c.Pull(context.Background(), "hello-world:latest"); err != nil {
		t.Fatal(err)
	}
	if err := c.RemoveImage(context.Background(), "hello-world:latest"); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 2 || ran[0] != "image pull --quiet hello-world:latest" || ran[1] != "image rm hello-world:latest" {
		t.Errorf("%q", ran)
	}
}

func TestPruningRemovesEveryUnusedImage(t *testing.T) {
	var ran []string
	if err := cli.New(script(map[string]string{"image prune": "Total reclaimed space: 0B\n"}, &ran)).PruneImages(context.Background()); err != nil || ran[0] != "image prune --all --force" {
		t.Errorf("%v %q", err, ran)
	}
}

func TestComposeVersionIsAskedShort(t *testing.T) {
	var ran []string
	v, err := cli.NewCompose(script(map[string]string{"compose version --short": "2.40.0\n"}, &ran)).Version(context.Background())
	if err != nil || v != "2.40.0" {
		t.Errorf("%q %v %q", v, err, ran)
	}
	if _, err := cli.NewCompose(script(nil, &ran)).Version(context.Background()); err == nil {
		t.Error("a docker without compose passed")
	}
}
