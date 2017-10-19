# ------------------------------------------------------------------------------------------------------------------------
# We are explicitly not using a templating language to inject the values as to encourage the user to limit their
# use of templating logic in these files. By design all injected values should be able to be set at runtime,
# and the shell script real work. If you need conditional logic, write it in bash or make another shell script.
# ------------------------------------------------------------------------------------------------------------------------


sudo rpm --import https://packages.cloud.google.com/yum/doc/yum-key.gpg
sudo rpm --import https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg

sudo sh -c 'cat <<EOF > /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=http://yum.kubernetes.io/repos/kubernetes-el7-x86_64
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg
       https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
EOF'

# SELinux is disabled in DO. This is not recommended and will be fixed later.

sudo yum makecache -y
sudo sudo yum install -y \
     docker \
     socat \
     ebtables \
     kubelet \
     kubeadm \
     epel-release

# jq needs its own special yum install as it depends on epel-release
sudo yum install -y jq

sudo systemctl enable docker
sudo systemctl enable kubelet
sudo systemctl start docker

# Required by kubeadm
sysctl -w net.bridge.bridge-nf-call-iptables=1
sysctl -p

TOKEN=$(cat /etc/kubicorn/cluster.json | jq -r '.values.itemMap.INJECTEDTOKEN')
MASTER=$(cat /etc/kubicorn/cluster.json | jq -r '.values.itemMap.INJECTEDMASTER')

sudo -E kubeadm reset
sudo -E kubeadm join --token ${TOKEN} ${MASTER}
