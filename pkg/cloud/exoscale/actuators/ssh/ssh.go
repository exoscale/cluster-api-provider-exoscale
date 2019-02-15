package machine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/exoscale/egoscale"
	"golang.org/x/crypto/ssh"
)

//RunCommand Run an ssh command
func (c *SSHClient) RunCommand(cmd string, stdout, stderr io.Writer) error {
	var err error

	retryOp := func() error {
		if c.Client, err = ssh.Dial("tcp", c.host, &ssh.ClientConfig{
			User:            c.user,
			Auth:            []ssh.AuthMethod{ssh.Password(c.password)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}); err != nil {
			return fmt.Errorf("unable to connect to cluster instance: %s", err)
		}

		sshSession, err := c.NewSession()
		if err != nil {
			return fmt.Errorf("unable to create SSH session: %s", err)
		}
		defer sshSession.Close()

		sshSession.Stdout = stdout
		sshSession.Stderr = stderr

		if err := sshSession.Run(cmd); err != nil {
			return err
		}

		return nil
	}

	if err = backoff.RetryNotify(
		retryOp,
		backoff.WithMaxRetries(backoff.NewConstantBackOff(10*time.Second), 6),
		func(_ error, d time.Duration) {
			fmt.Printf("! Cluster instance not ready yet, retrying in %s...\n", d)
		}); err != nil {
		return err
	}

	return nil
}

//QuickCommand run quick command over ssh and get result as string
func (c *SSHClient) QuickCommand(command string) (string, error) {
	var buf bytes.Buffer

	if err := c.RunCommand(command, &buf, nil); err != nil {
		return "", err
	}
	return buf.String(), nil
}

//Scp like scp command
func (c *SSHClient) Scp(src, dst string) error {
	var buf bytes.Buffer

	if err := c.RunCommand(fmt.Sprintf("sudo cat %s", src), &buf, nil); err != nil {
		return err
	}

	if _, err := os.Stat(path.Dir(dst)); os.IsNotExist(err) {
		if err := os.MkdirAll(path.Dir(dst), os.ModePerm); err != nil {
			return fmt.Errorf("unable to create directory %q: %s", path.Dir(dst), err)
		}
	}

	return ioutil.WriteFile(dst, buf.Bytes(), 0600)
}

// SSHClient represent an ssh client
type SSHClient struct {
	*ssh.Client
	host     string
	password string
	user     string
}

// NewSSHClient create a new ssh client
func NewSSHClient(host, hostUser, password string) *SSHClient {
	return &SSHClient{
		Client:   nil,
		host:     host + ":22",
		password: password,
		user:     hostUser,
	}
}

//CreateSSHKey create an ssh key pair
func CreateSSHKey(ctx context.Context, client *egoscale.Client, name string) (*egoscale.SSHKeyPair, error) {
	resp, err := client.RequestWithContext(ctx, &egoscale.CreateSSHKeyPair{
		Name: name,
	})
	if err != nil {
		return nil, err
	}

	sshKeyPair, ok := resp.(*egoscale.SSHKeyPair)
	if !ok {
		return nil, fmt.Errorf("wrong type expected %q, got %T", "egoscale.CreateSSHKeyPairResponse", resp)
	}

	return sshKeyPair, nil
}
