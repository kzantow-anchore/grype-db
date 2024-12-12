package distribution

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anchore/grype-db/internal/log"
	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/registry"
	"io"
	"net"
	"os/exec"
	"strings"
	"testing"
)

func Test_oras(t *testing.T) {
	registryHost := RunRegistry(t)

	tests := []struct {
		name    string
		url     string
		blob    []byte
		wantErr require.ErrorAssertionFunc
	}{
		{
			name: "local-registry-with-tag",
			blob: []byte(`some-contents`),
			url:  fmt.Sprintf("%v/%v:%v", registryHost, "some/thing", "withtag"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			os := OrasSource{
				PlainHTTP: true,
			}
			ctx := context.TODO()

			err := os.Push(ctx, test.url, "application/grype-db", bytes.NewReader(test.blob))
			if test.wantErr != nil {
				test.wantErr(t, err)
			} else {
				require.NoError(t, err)
			}

			//ref := test .url + "@" + digest // ??

			contents := bytes.Buffer{}
			rdr, err := os.Pull(ctx, test.url)
			require.NoError(t, err)
			_, err = io.Copy(&contents, rdr)
			require.NoError(t, err)
			require.Equal(t, test.blob, contents.Bytes())
		})
	}
}

func RunRegistry(t *testing.T) (url string) {
	if true {
		hostPort, err := GetFreePort()
		if err != nil {
			t.Fatal(err)
		}
		_ = RunContainerDaemon(t, "registry:2", "", fmt.Sprintf("-p %v:5000", hostPort))
		url = fmt.Sprintf(`localhost:%v`, hostPort)
		t.Logf("registry at: %v", url)
		return url
	}

	ctx := context.TODO()
	registryContainer, err := registry.Run(ctx, "registry:2.8.3")
	defer func() {
		if err := testcontainers.TerminateContainer(registryContainer); err != nil {
			log.Warnf("failed to terminate container: %s", err)
		}
	}()
	if err != nil {
		log.Warnf("failed to start container: %s", err)
		return
	}

	//registryContainer.StartLogProducer(ctx, testcontainers.TestLogger(t)
	err = registryContainer.Start(ctx)
	require.NoError(t, err)

	url, err = registryContainer.Address(ctx)
	require.NoError(t, err)
	if strings.HasPrefix(url, "https://") {
		url = strings.TrimPrefix(url, "https://")
	}
	if strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "http://")
	}
	return url
}

func RunContainerDaemon(t *testing.T, image, name string, args string) (id string) {
	if name != "" {
		name = "--name '" + name + "'"
	}
	//id, _, _, err := Run(t, fmt.Sprintf("docker run --rm -d %s %s '%s'", args, name, image))
	id, _, _, err := Run(t, fmt.Sprintf("docker run --rm -i %s %s '%s'", args, name, image))
	t.Cleanup(func() {
		_, _, _, _ = Run(t, "docker stop "+id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func Run(t *testing.T, command string) (stdout string, stderr string, retCode int, err error) {
	parts, err := shellquote.Split(command)
	if err != nil {
		t.Fatal(err)
	}

	// run the command
	cmd := exec.Command(parts[0], parts[1:]...)
	stdoutBuf := bytes.Buffer{}
	stderrBuf := bytes.Buffer{}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// run the command
	err = cmd.Run()

	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	if exitCode != 0 {
		t.Logf("Ran: %v got exitCode: %v, stdout: \n%v\n stderr: \n%v", parts, exitCode, stdoutBuf.String(), stderrBuf.String())
	}

	return stdoutBuf.String(), stderrBuf.String(), exitCode, err
}

// GetFreePort asks the kernel for a free open port that is ready to use.
func GetFreePort() (port int, err error) {
	var a *net.TCPAddr
	if a, err = net.ResolveTCPAddr("tcp", "localhost:0"); err == nil {
		var l *net.TCPListener
		if l, err = net.ListenTCP("tcp", a); err == nil {
			defer closeOrLog("tcp", l)
			return l.Addr().(*net.TCPAddr).Port, nil
		}
	}
	return
}
