package imagebuilder

import "golang.org/x/crypto/ssh"

type Cloud interface {
	GetInstance() (Instance, error)
	CreateInstance() (Instance, error)

	FindImage(imageName string) (Image, error)

	GetExtraEnv() (map[string]string, error)
}

type Instance interface {
	DialSSH(config *ssh.ClientConfig) (*ssh.Client, error)
	Shutdown() error
}

type Image interface {
	EnsurePublic() error

	// Adds the specified tags to the image
	AddTags(tags map[string]string) error

	ReplicateImage(makePublic bool) (map[string]Image, error)
}
