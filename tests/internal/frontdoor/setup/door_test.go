// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package setup_test

// Verifies:
//   - /setup is a script for limen as the host reached it, naming each binary's checksum
//   - /get hands out the binaries limen carries, and nothing else
//   - A Host header that is not a plain host:port is refused; a limen with no binaries says so
//   - The script, run by sh, fetches the binary, checks it, runs it with its arguments (--limen
//     added where taken) and removes it; a binary that does not match its checksum is not run
//   - /setup.ps1 names the Windows binary's checksum, which /setup leaves out; run by PowerShell
//     (iex or a script block), it runs limenctl with its arguments and removes it

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nauarch/limen/internal/frontdoor/setup"
)

// fakeEnv makes this test binary act as limenctl for the PowerShell script: it prints the
// arguments it was given.
const fakeEnv = "LIMEN_FAKE_LIMENCTL"

func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) != "" {
		fmt.Println("limenctl " + strings.Join(os.Args[1:], " "))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeLimenctl is a script standing in for limenctl: it prints the arguments it was given.
const fakeLimenctl = "#!/bin/sh\necho \"limenctl $*\"\n"

// platform is this machine's, as the script names it.
func platform() string {
	arch := map[string]string{"amd64": "amd64", "arm64": "arm64"}[runtime.GOARCH]
	return runtime.GOOS + "-" + arch
}

// serve is limen's setup door carrying binaries.
func serve(t *testing.T, binaries map[string]string) *httptest.Server {
	files := fstest.MapFS{}
	for name, data := range binaries {
		files[name] = &fstest.MapFile{Data: []byte(data)}
	}
	s := httptest.NewServer(setup.New(setup.Options{Files: files}))
	t.Cleanup(s.Close)
	return s
}

// get answers a GET, its status and body.
func get(t *testing.T, url string, header map[string]string) (int, string) {
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for k, v := range header {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestTheScriptNamesLimenAndTheChecksums(t *testing.T) {
	s := serve(t, map[string]string{"limenctl-linux-amd64": "amd", "limenctl-linux-arm64": "arm", "README.txt": "x"})
	code, script := get(t, s.URL+"/setup", map[string]string{"Host": "192.0.2.10:5000"})
	sum := sha256.Sum256([]byte("amd"))
	if code != http.StatusOK || !strings.Contains(script, "limen='http://192.0.2.10:5000'") || !strings.Contains(script, "linux-amd64) sum="+hex.EncodeToString(sum[:])) || !strings.Contains(script, "linux-arm64) sum=") {
		t.Errorf("%d\n%s", code, script)
	}
	if code, body := get(t, s.URL+"/get/limenctl-linux-arm64", nil); code != http.StatusOK || body != "arm" {
		t.Errorf("binary: %d %q", code, body)
	}
	if code, _ := get(t, s.URL+"/get/README.txt", nil); code != http.StatusNotFound {
		t.Errorf("other files: %d", code)
	}
}

func TestOddHostsAndMissingBinariesAreRefused(t *testing.T) {
	s := serve(t, map[string]string{"limenctl-linux-amd64": "amd"})
	for _, h := range []string{"limen:5000';rm -rf /;'", "a b", "$(id)"} {
		if code, _ := get(t, s.URL+"/setup", map[string]string{"Host": h}); code != http.StatusBadRequest {
			t.Errorf("%q: %d", h, code)
		}
	}
	if code, _ := get(t, s.URL+"/setup", map[string]string{"Host": "[fd00::2]:5000"}); code != http.StatusOK {
		t.Errorf("an IPv6 host: %d", code)
	}
	empty := serve(t, nil)
	if code, body := get(t, empty.URL+"/setup", nil); code != http.StatusServiceUnavailable || !strings.Contains(body, "carries no limenctl") {
		t.Errorf("no binaries: %d %s", code, body)
	}
}

func TestTheScriptRunsLimenctlAndCleansUp(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs a POSIX sh, sha256sum and uname")
	}
	s := serve(t, map[string]string{"limenctl-" + platform(): fakeLimenctl})
	_, script := get(t, s.URL+"/setup", nil)
	tmp := t.TempDir()
	// The script wants root; run it as is where the test is root, else with id faked.
	fakeBin := filepath.Join(tmp, "bin")
	_ = os.MkdirAll(fakeBin, 0o755)
	_ = os.WriteFile(filepath.Join(fakeBin, "id"), []byte("#!/bin/sh\necho 0\n"), 0o755)
	run := func(args ...string) (string, error) {
		cmd := exec.Command(sh, append([]string{"-s", "--"}, args...)...)
		cmd.Stdin = strings.NewReader(script)
		cmd.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "TMPDIR="+tmp)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("configure", "--yes")
	if err != nil || strings.TrimSpace(out) != "limenctl configure --limen "+s.URL+" --yes" {
		t.Errorf("%v: %q", err, out)
	}
	if out, _ := run(); strings.TrimSpace(out) != "limenctl plan --limen "+s.URL {
		t.Errorf("no arguments: %q", out)
	}
	if out, _ := run("rollback"); strings.TrimSpace(out) != "limenctl rollback" {
		t.Errorf("rollback: %q", out)
	}
	left, _ := filepath.Glob(filepath.Join(tmp, "tmp.*"))
	if len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
	tampered := strings.Replace(script, "sum=", "sum=00", 1)
	cmd := exec.Command(sh, "-s", "--", "plan")
	cmd.Stdin = strings.NewReader(tampered)
	cmd.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "TMPDIR="+tmp)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "does not match its checksum") {
		t.Errorf("a bad checksum ran: %v %s", err, out)
	}
}

func TestThePowerShellScriptNamesTheWindowsBinary(t *testing.T) {
	s := serve(t, map[string]string{"limenctl-linux-amd64": "amd", setup.Windows: "win"})
	code, script := get(t, s.URL+"/setup.ps1", map[string]string{"Host": "192.0.2.10:5000"})
	sum := sha256.Sum256([]byte("win"))
	if code != http.StatusOK || !strings.Contains(script, "$limen = 'http://192.0.2.10:5000'") || !strings.Contains(script, hex.EncodeToString(sum[:])) {
		t.Errorf("%d\n%s", code, script)
	}
	if _, sh := get(t, s.URL+"/setup", nil); strings.Contains(sh, "windows") {
		t.Errorf("the sh script names the Windows binary:\n%s", sh)
	}
	if code, _ := get(t, s.URL+"/setup.ps1", map[string]string{"Host": "a'b"}); code != http.StatusBadRequest {
		t.Errorf("an odd host: %d", code)
	}
	if code, _ := get(t, serve(t, map[string]string{"limenctl-linux-amd64": "amd"}).URL+"/setup.ps1", nil); code != http.StatusServiceUnavailable {
		t.Errorf("no Windows binary: %d", code)
	}
}

func TestThePowerShellScriptRunsLimenctlAndCleansUp(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		pwsh, err = exec.LookPath("powershell")
	}
	if err != nil || runtime.GOOS != "windows" {
		t.Skip("needs PowerShell on Windows")
	}
	self, err := os.ReadFile(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	s := serve(t, map[string]string{setup.Windows: string(self)})
	tmp := t.TempDir()
	run := func(command string) string {
		cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-Command", command)
		cmd.Env = append(os.Environ(), fakeEnv+"=1", "TMP="+tmp, "TEMP="+tmp)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Errorf("%s: %v\n%s", command, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if out := run("irm " + s.URL + "/setup.ps1 | iex"); out != "limenctl plan --limen "+s.URL {
		t.Errorf("iex: %q", out)
	}
	if out := run("& ([scriptblock]::Create((irm " + s.URL + "/setup.ps1))) configure --yes"); out != "limenctl configure --limen "+s.URL+" --yes" {
		t.Errorf("script block: %q", out)
	}
	if out := run("& ([scriptblock]::Create((irm " + s.URL + "/setup.ps1))) verify"); out != "limenctl verify --limen "+s.URL {
		t.Errorf("verify: %q", out)
	}
	if out := run("& ([scriptblock]::Create((irm " + s.URL + "/setup.ps1))) unconfigure"); out != "limenctl unconfigure" {
		t.Errorf("unconfigure: %q", out)
	}
	if left, _ := filepath.Glob(filepath.Join(tmp, "limen-*")); len(left) != 0 {
		t.Errorf("left behind: %v", left)
	}
}
