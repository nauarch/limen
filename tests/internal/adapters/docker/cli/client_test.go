// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package cli_test

// Verifies:
//   - docker info is read: version, store, root, containerd, platform, swarm, userns-remap, mirrors
//   - Containers are read through ps then inspect: name, image, labels, host network, host ports
//     and what each wrote outside its volumes
//   - Images are read through ls then inspect, each ID once
//   - No containers or images skips the inspect; a failing or garbled docker is an error

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/nauarch/limen/internal/adapters/docker/cli"
	"github.com/nauarch/limen/internal/kernel/errkind"
)

// script answers each docker command line from a table by prefix, recording what was run.
func script(answers map[string]string, ran *[]string) cli.Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		line := strings.Join(args, " ")
		*ran = append(*ran, line)
		for prefix, out := range answers {
			if strings.HasPrefix(line, prefix) {
				return []byte(out), nil
			}
		}
		return nil, errors.New("unscripted: " + line)
	}
}

const infoJSON = `{"ServerVersion":"29.9.2","Driver":"overlayfs","DriverStatus":[["driver-type","io.containerd.snapshotter.v1"]],
"DockerRootDir":"/var/lib/docker","OSType":"linux","Architecture":"x86_64","Containerd":{"Address":"/run/containerd/containerd.sock"},"SecurityOptions":["name=seccomp,profile=builtin","name=userns"],
"Swarm":{"LocalNodeState":"active"},"RegistryConfig":{"Mirrors":["http://10.0.0.5:5000/"]}}`

func TestInfoIsRead(t *testing.T) {
	var ran []string
	i, err := cli.New(script(map[string]string{"info": infoJSON}, &ran)).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i.Containerd != "/run/containerd/containerd.sock" || i.OS != "linux" || i.Arch != "x86_64" {
		t.Errorf("containerd %q, platform %s/%s", i.Containerd, i.OS, i.Arch)
	}
	if i.Version != "29.9.2" || i.Driver != "overlayfs" || !i.Snapshotter || i.RootDir != "/var/lib/docker" || !i.Swarm || !i.UsernsRemap || !slices.Equal(i.Mirrors, []string{"http://10.0.0.5:5000/"}) {
		t.Errorf("%+v", i)
	}
	classic, err := cli.New(script(map[string]string{"info": `{"Driver":"overlay2","Swarm":{"LocalNodeState":"inactive"}}`}, &ran)).Info(context.Background())
	if err != nil || classic.Snapshotter || classic.Swarm || classic.UsernsRemap {
		t.Errorf("classic %+v %v", classic, err)
	}
}

func TestContainersAreRead(t *testing.T) {
	var ran []string
	c := cli.New(script(map[string]string{
		"ps --all": "c1\nc2\n",
		"container inspect --size c1 c2": `[
{"Id":"c1","Name":"/resolver","Image":"sha256:a","SizeRw":4096,"Config":{"Image":"coredns/coredns:latest","Labels":{"com.docker.compose.project":"dns"}},
 "State":{"Running":true},"HostConfig":{"NetworkMode":"host"}},
{"Id":"c2","Name":"/portainer_agent","Image":"sha256:p","Config":{"Image":"portainer/agent"},"State":{"Running":false},
 "HostConfig":{"NetworkMode":"bridge","PortBindings":{"9001/tcp":[{"HostIp":"","HostPort":"9001"}],"53/udp":[{"HostPort":""}]}}}]`,
	}, &ran))
	got, err := c.Containers(context.Background())
	if err != nil || len(got) != 2 {
		t.Fatalf("%+v %v", got, err)
	}
	a, p := got[0], got[1]
	if a.Name != "resolver" || a.Image != "coredns/coredns:latest" || a.ImageID != "sha256:a" || !a.Running || !a.HostNetwork || a.Labels["com.docker.compose.project"] != "dns" {
		t.Errorf("resolver %+v", a)
	}
	if a.Written != 4096 || p.Written != -1 {
		t.Errorf("written %d, %d", a.Written, p.Written)
	}
	if p.Running || p.HostNetwork || !slices.Equal(p.Ports, []int{9001}) {
		t.Errorf("portainer %+v", p)
	}
}

func TestImagesAreReadOnceEach(t *testing.T) {
	var ran []string
	c := cli.New(script(map[string]string{
		"image ls":                        "sha256:a\nsha256:a\nsha256:b\n",
		"image inspect sha256:a sha256:b": `[{"Id":"sha256:a","RepoTags":["alpine:3"],"RepoDigests":["alpine@sha256:d"],"Size":8000000},{"Id":"sha256:b","Size":5}]`,
	}, &ran))
	got, err := c.Images(context.Background())
	if err != nil || len(got) != 2 || got[0].Tags[0] != "alpine:3" || got[0].Digests[0] != "alpine@sha256:d" || got[0].Size != 8000000 {
		t.Errorf("%+v %v (ran %v)", got, err, ran)
	}
}

func TestNothingToInspectSkipsTheInspect(t *testing.T) {
	var ran []string
	c := cli.New(script(map[string]string{"ps": "", "image ls": "\n"}, &ran))
	cs, cerr := c.Containers(context.Background())
	is, ierr := c.Images(context.Background())
	if cs != nil || is != nil || cerr != nil || ierr != nil || len(ran) != 2 {
		t.Errorf("%v %v %v %v ran %v", cs, is, cerr, ierr, ran)
	}
}

func TestAFailingOrGarbledDockerIsAnError(t *testing.T) {
	var ran []string
	if _, err := cli.New(script(nil, &ran)).Info(context.Background()); err == nil {
		t.Error("failing docker read")
	}
	if _, err := cli.New(script(map[string]string{"info": "not json"}, &ran)).Info(context.Background()); errkind.Of(err) != errkind.Upstream {
		t.Errorf("garbled info: %v", err)
	}
	c := cli.New(script(map[string]string{"ps": "c1", "container inspect": "{"}, &ran))
	if _, err := c.Containers(context.Background()); errkind.Of(err) != errkind.Upstream {
		t.Errorf("garbled inspect: %v", err)
	}
}
