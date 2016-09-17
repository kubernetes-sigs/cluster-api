package imagebuilder

type Config struct {
	Cloud         string
	TemplatePath  string
	SetupCommands []string

	BootstrapVZRepo   string
	BootstrapVZBranch string

	SSHUsername   string
	SSHPublicKey  string
	SSHPrivateKey string

	// Tags to add to the image
	Tags map[string]string
}

func (c *Config) InitDefaults() {
	// At least until https://github.com/andsens/bootstrap-vz/pull/316 merges
	// Maybe we should have our own fork anyway though
	c.BootstrapVZRepo = "https://github.com/justinsb/bootstrap-vz.git"
	c.BootstrapVZBranch = "master"

	c.SSHUsername = "admin"
	c.SSHPublicKey = "~/.ssh/id_rsa.pub"
	c.SSHPrivateKey = "~/.ssh/id_rsa"

	c.SetupCommands = []string{
		"sudo apt-get update",
		"sudo apt-get install --yes git python debootstrap python-pip kpartx parted",
		"sudo pip install termcolor jsonschema fysom docopt pyyaml boto",
	}
}

type AWSConfig struct {
	Config

	Region          string
	ImageID         string
	InstanceType    string
	SSHKeyName      string
	SubnetID        string
	SecurityGroupID string
}

func (c *AWSConfig) InitDefaults() {
	c.Config.InitDefaults()
	c.Region = "us-east-1"
	// Debian 8.4 us-east-1 from https://wiki.debian.org/Cloud/AmazonEC2Image/Jessie
	c.ImageID = "ami-c8bda8a2"
	c.InstanceType = "m3.medium"
}

type GCEConfig struct {
	Config

	// To create an image on GCE, we have to upload it to a bucket first
	GCSDestination string

	Project     string
	Zone        string
	MachineName string

	MachineType string
	Image       string
}

func (c *GCEConfig) InitDefaults() {
	c.Config.InitDefaults()
	c.MachineName = "k8s-imagebuilder"
	c.Zone = "us-central1-f"
	c.MachineType = "n1-standard-2"
	c.Image = "https://www.googleapis.com/compute/v1/projects/debian-cloud/global/images/debian-8-jessie-v20160329"
}
