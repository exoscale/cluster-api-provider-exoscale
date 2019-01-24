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
		if c.c, err = ssh.Dial("tcp", c.host, &ssh.ClientConfig{
			User:            c.user,
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(c.hostKey)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}); err != nil {
			return fmt.Errorf("unable to connect to cluster instance: %s", err)
		}

		sshSession, err := c.c.NewSession()
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

//SSHClient represent an ssh client
type SSHClient struct {
	host    string
	hostKey ssh.Signer
	user    string
	c       *ssh.Client
}

//NewSSHClient create a new ssh client
func NewSSHClient(host, hostUser, privateKey string) (*SSHClient, error) {
	var c = SSHClient{
		host: host + ":22",
		user: hostUser,
	}

	var err error
	if c.hostKey, err = ssh.ParsePrivateKey([]byte(privateKey)); err != nil {
		return nil, fmt.Errorf("unable to parse cluster instance SSH private key: %s", err)
	}

	return &c, nil
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
