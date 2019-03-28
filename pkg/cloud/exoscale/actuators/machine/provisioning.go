package machine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/exoscale/egoscale"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"
	ssh "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/ssh"
	tokens "sigs.k8s.io/cluster-api-provider-exoscale/pkg/cloud/exoscale/actuators/tokens"
	clusterv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"
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

// provisioningSteps represents an instance provisioning steps for k8s
var provisioningSteps = []kubeBootstrapStep{
	{
		name: "Instance system upgrade",
		command: `\
set -xe

sudo -E DEBIAN_FRONTEND=noninteractive apt-get update
sudo -E DEBIAN_FRONTEND=noninteractive apt-get -o Dpkg::Options::="--force-confnew" upgrade -y
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
	},
}

// masterBootstapSteps represents a k8s instance bootstrap steps
var masterBootstapSteps = kubeBootstrapStep{

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
}

// nodeJoinSteps represents a k8s node join steps
var nodeJoinSteps = kubeBootstrapStep{
	name: "Kubernetes cluster node initialization",
	command: `\
set -xe

sudo kubeadm join \
	--token {{ .Token }} {{ .MasterIP }}:{{ .MasterPort }} \
	--discovery-token-unsafe-skip-ca-verification`,
}

type kubeCluster struct {
	Name              string
	KubernetesVersion string
	DockerVersion     string
	CalicoVersion     string
	Address           string
	Token             string
	MasterIP          string
	MasterPort        string
	Sha256Hash        string
}

func bootstrapCluster(sshClient *ssh.SSHClient, cluster kubeCluster, master, debug bool) error {
	provStep := make([]kubeBootstrapStep, len(provisioningSteps))
	copy(provStep, provisioningSteps)
	if master {
		provStep = append(provStep, masterBootstapSteps)
	} else {
		provStep = append(provStep, nodeJoinSteps)
	}
	for _, step := range provStep {
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

	return nil
}

func (*Actuator) provisionMaster(machine *clusterv1.Machine, vm *egoscale.VirtualMachine, username string) error {
	sshClient := ssh.NewSSHClient(
		vm.IP().String(),
		username,
		vm.Password,
	)

	test := kubeCluster{
		Name:              vm.Name,
		KubernetesVersion: machine.Spec.Versions.ControlPlane,
		CalicoVersion:     kubeCalicoVersion,
		DockerVersion:     kubeDockerVersion,
		Address:           vm.IP().String(),
	}

	if err := bootstrapCluster(sshClient, test, true, false); err != nil {
		return fmt.Errorf("cluster bootstrap failed: %s", err)
	}
	return nil
}

func (a *Actuator) provisionNode(cluster *clusterv1.Cluster, machine *clusterv1.Machine, vm *egoscale.VirtualMachine, username string) error {
	bootstrapToken, err := a.getNodeJoinToken(cluster, machine)
	if err != nil {
		return fmt.Errorf("failed to obtain token for node %q to join cluster %q: %v", machine.Name, cluster.Name, err)
	}

	//XXX work only with 1 master at the moment
	controlPlaneMachine, err := a.getControlPlaneMachine(machine)
	if err != nil {
		return err
	}

	controlPlaneIP, err := a.GetIP(cluster, controlPlaneMachine)
	if err != nil {
		return fmt.Errorf("failed to retrieve controlplane (GetIP): %v", err)
	}

	sshClient := ssh.NewSSHClient(
		vm.IP().String(),
		username,
		vm.Password,
	)

	if err := bootstrapCluster(sshClient, kubeCluster{
		Name:              vm.Name,
		KubernetesVersion: machine.Spec.Versions.Kubelet,
		DockerVersion:     kubeDockerVersion,
		Address:           vm.IP().String(),
		MasterIP:          controlPlaneIP,
		Token:             bootstrapToken,
		MasterPort:        "6443",
	}, false, false); err != nil {
		return fmt.Errorf("node bootstrap failed: %s", err)
	}
	return nil
}

// getControlPlaneMachine get the first controlPlane machine found
func (a *Actuator) getControlPlaneMachine(machine *clusterv1.Machine) (*clusterv1.Machine, error) {
	machineClient := a.machinesGetter.Machines(machine.Namespace)
	machineList, err := machineClient.List(v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed get machine list: %v", err)
	}
	controlPlaneList := a.getControlPlaneMachines(machineList)

	//XXX work only with 1 master at the moment
	if len(controlPlaneList) == 1 {
		return controlPlaneList[0], nil
	}

	return nil, fmt.Errorf("invalid master number expect 1 (XXX for the time being) got %d", len(controlPlaneList))
}

func (a *Actuator) getNodeJoinToken(cluster *clusterv1.Cluster, machine *clusterv1.Machine) (string, error) {

	//XXX work only with 1 master at the moment
	controlPlaneMachine, err := a.getControlPlaneMachine(machine)
	if err != nil {
		return "", err
	}

	controlPlaneIP, err := a.GetIP(cluster, controlPlaneMachine)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve controlplane (GetIP): %v", err)
	}

	controlPlaneURL := fmt.Sprintf("https://%s:6443", controlPlaneIP)
	klog.V(1).Infof("control plane url %q", controlPlaneURL)

	kubeConfig, err := a.GetKubeConfig(cluster, controlPlaneMachine)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve kubeconfig for cluster %q: %v", cluster.Name, err)
	}

	clientConfig, err := clientcmd.BuildConfigFromKubeconfigGetter(controlPlaneURL, func() (*clientcmdapi.Config, error) {
		return clientcmd.Load([]byte(kubeConfig))
	})

	if err != nil {
		return "", fmt.Errorf("failed to get client config for cluster at %q: %v", controlPlaneURL, err)
	}

	coreClient, err := corev1.NewForConfig(clientConfig)
	if err != nil {
		return "", fmt.Errorf("failed to initialize new corev1 client: %v", err)
	}

	// XXX this could be super slow...
	bootstrapToken, err := tokens.NewBootstrap(coreClient, 20*time.Minute)
	if err != nil {
		return "", fmt.Errorf("failed to create new bootstrap token: %v", err)
	}

	klog.V(1).Infof("boostrap token %q", bootstrapToken)

	return bootstrapToken, nil
}
