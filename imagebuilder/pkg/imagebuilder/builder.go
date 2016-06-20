package imagebuilder

import (
	"bytes"
	"fmt"
	"math/rand"
	"path"
)

type Builder struct {
	config *Config
	ssh    *SSH
}

func NewBuilder(config *Config, ssh *SSH) *Builder {
	return &Builder{
		config: config,
		ssh:    ssh,
	}
}

func (b *Builder) RunSetupCommands() error {
	for _, c := range b.config.SetupCommands {
		if err := b.ssh.Exec(c); err != nil {
			return err
		}
	}

	return nil
}

func (b *Builder) BuildImage(template []byte, extraEnv map[string]string) error {
	tmpdir := fmt.Sprintf("/tmp/imagebuilder-%d", rand.Int63())
	err := b.ssh.SCPMkdir(tmpdir, 0755)
	if err != nil {
		return err
	}
	defer b.ssh.Exec("rm -rf " + tmpdir)

	logdir := path.Join(tmpdir, "logs")
	err = b.ssh.SCPMkdir(logdir, 0755)
	if err != nil {
		return err
	}

	//err = ssh.Exec("git clone https://github.com/andsens/bootstrap-vz.git " + tmpdir + "/bootstrap-vz")
	err = b.ssh.Exec("git clone " + b.config.BootstrapVZRepo + " -b " + b.config.BootstrapVZBranch + " " + tmpdir + "/bootstrap-vz")
	if err != nil {
		return err
	}

	err = b.ssh.SCPPut(tmpdir+"/template.yml", len(template), bytes.NewReader(template), 0644)
	if err != nil {
		return err
	}

	// TODO: Create dir for logs, log to that dir using --log, collect logs from that dir
	cmd := b.ssh.Command(fmt.Sprintf("./bootstrap-vz/bootstrap-vz --debug --log %q ./template.yml", logdir))
	cmd.Cwd = tmpdir
	for k, v := range extraEnv {
		cmd.Env[k] = v
	}
	cmd.Sudo = true
	err = cmd.Run()
	if err != nil {
		return err
	}

	// TODO: Capture debug output file?
	return nil
}
