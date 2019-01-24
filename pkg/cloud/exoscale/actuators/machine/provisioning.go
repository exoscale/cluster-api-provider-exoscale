package machine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"text/template"

	ssh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"
)

const (
	// kubeDockerVersion is the version installed on Ubuntu Xenial
	kubeDockerVersion = "18.06"

	// kubeCalicoVersion is the version of Calico installed
	kubeCalicoVersion = "3.4"
)

// kubeBootstrapStep represents a k8s instance bootstrap step
type kubeBootstrapStep struct {
	name    string
	command string
}

// kubeBootstrapSteps represents a k8s instance bootstrap steps
var kubeBootstrapSteps = []kubeBootstrapStep{
	{
		name: "Instance system upgrade",
		command: `\
set -xe

sudo -E DEBIAN_FRONTEND=noninteractive apt-get update
sudo -E DEBIAN_FRONTEND=noninteractive apt-get upgrade -y
sudo -E DEBIAN_FRONTEND=noninteractive apt-get install -y \
	apt-transport-https \
	ca-certificates \
	curl \
	golang-cfssl \
	software-properties-common
nohup sh -c 'sleep 5s ; sudo reboot' &
exit`,
	},
	{
		name: "Docker Engine installation",
		command: `\
set -xe

curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -

sudo add-apt-repository \
	"deb [arch=amd64] https://download.docker.com/linux/ubuntu \
	$(lsb_release -cs) \
	stable"

sudo -E DEBIAN_FRONTEND=noninteractive apt-get update

PKG_VERSION=$(apt-cache madison docker-ce | awk '$3 ~ /{{ .DockerVersion }}/ { print $3 }' | sort -t : -k 2 -n | tail -n 1)
if [[ -z "${PKG_VERSION}" ]]; then
	echo "error: unable to find docker-ce package for version {{ .DockerVersion }}" >&2
	exit 1
fi

sudo -E DEBIAN_FRONTEND=noninteractive apt-get install -y docker-ce=${PKG_VERSION}
sudo apt-mark hold docker-ce

cat <<EOF > csr.json
{
	"hosts": ["{{ .Address }}"],
	"key": {"algo": "rsa", "size": 2048},
	"names": [{"C": "CH", "L": "Lausanne", "O": "Exoscale", "OU": "exokube", "ST": ""}]
}
EOF

cfssl genkey -initca csr.json | cfssljson -bare ca

cfssl gencert \
	-ca ca.pem \
	-ca-key ca-key.pem \
	-hostname {{ .Address }} csr.json | cfssljson -bare

cat <<EOF | sudo tee /etc/docker/daemon.json
{
	"hosts": ["unix:///var/run/docker.sock", "tcp://0.0.0.0:2376"],
	"tlsverify": true,
	"tlscacert": "/etc/docker/ca.pem",
	"tlscert": "/etc/docker/cert.pem",
	"tlskey": "/etc/docker/key.pem",
	"exec-opts": ["native.cgroupdriver=systemd"],
	"storage-driver": "overlay2",
	"log-driver": "json-file",
	"log-opts": {
		"max-size": "100m"
	}
}
EOF

sudo mv ca.pem /etc/docker/ca.pem
sudo mv cert.pem /etc/docker/cert.pem
sudo mv cert-key.pem /etc/docker/key.pem
rm -f *.{csr,json,pem}

sudo mkdir /etc/systemd/system/docker.service.d/
cat <<EOF | sudo tee /etc/systemd/system/docker.service.d/override.conf
[Service]
ExecStart=
ExecStart=/usr/bin/dockerd
EOF
sudo systemctl daemon-reload \
 && sudo systemctl restart docker`,
	},
	{
		name: "Kubernetes cluster node installation", command: `\
set -xe

curl -fsSL https://packages.cloud.google.com/apt/doc/apt-key.gpg | sudo apt-key add -
cat <<EOF | sudo tee /etc/apt/sources.list.d/kubernetes.list
deb http://apt.kubernetes.io/ kubernetes-xenial main
EOF
sudo -E DEBIAN_FRONTEND=noninteractive apt-get update

PKG_VERSION=$(apt-cache madison kubelet | awk '$3 ~ /{{ .KubernetesVersion }}-/ { print $3 }' | sort -t "-" -k 2 -n | tail -n 1)
if [[ -z "${PKG_VERSION}" ]]; then
	echo "error: unable to find kubelet package for version {{ .KubernetesVersion }}" >&2
	exit 1
fi

sudo -E DEBIAN_FRONTEND=noninteractive apt-get install -y kubelet=${PKG_VERSION} \
	kubeadm=${PKG_VERSION} \
	kubectl=${PKG_VERSION}
sudo apt-mark hold kubelet kubeadm kubectl`,
	}, {
		name: "Kubernetes cluster node initialization",
		command: `\
set -xe

sudo kubeadm init \
	--pod-network-cidr=192.168.0.0/16 \
	--kubernetes-version "{{ .KubernetesVersion }}"
sudo kubectl --kubeconfig=/etc/kubernetes/admin.conf taint nodes --all node-role.kubernetes.io/master-
sudo kubectl --kubeconfig=/etc/kubernetes/admin.conf apply \
		-f https://docs.projectcalico.org/v{{ .CalicoVersion }}/getting-started/kubernetes/installation/hosted/etcd.yaml
sudo kubectl --kubeconfig=/etc/kubernetes/admin.conf apply \
		-f https://docs.projectcalico.org/v{{ .CalicoVersion }}/getting-started/kubernetes/installation/hosted/calico.yaml`,
	},
}

type kubeCluster struct {
	Name              string
	KubernetesVersion string
	DockerVersion     string
	CalicoVersion     string
	Address           string
}

func bootstrapExokubeCluster(sshClient *ssh.SSHClient, cluster kubeCluster, debug bool) error {
	for _, step := range kubeBootstrapSteps {
		var (
			stdout, stderr io.Writer
			cmd            bytes.Buffer
			errBuf         bytes.Buffer
		)

		stderr = &errBuf
		if debug {
			stdout = os.Stderr
			stderr = os.Stderr
		}

		err := template.Must(template.New("command").Parse(step.command)).Execute(&cmd, cluster)
		if err != nil {
			return fmt.Errorf("template error: %s", err)
		}

		fmt.Printf("%s... ", step.name)

		if err := sshClient.RunCommand(cmd.String(), stdout, stderr); err != nil {
			fmt.Printf("\n%s: failed\n", step.name)

			if errBuf.Len() > 0 {
				fmt.Println(errBuf.String())
			}

			return err
		}

		fmt.Printf("success!\n")
	}

	// for _, file := range []string{"ca.pem", "cert.pem", "key.pem"} {
	// 	err := sshClient.scp("/etc/docker/"+file, path.Join(getKubeconfigPath(cluster.Name), "docker", file))
	// 	if err != nil {
	// 		return fmt.Errorf("unable to retrieve Docker host file %q: %s", file, err)
	// 	}
	// }

	// err := sshClient.scp("/etc/kubernetes/admin.conf", path.Join(getKubeconfigPath(cluster.Name), "kubeconfig"))
	// if err != nil {
	// 	return fmt.Errorf("unable to retrieve Kubernetes cluster configuration: %s", err)
	// }

	return nil
}
