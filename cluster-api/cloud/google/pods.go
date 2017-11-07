package google

import (
	"bytes"
	"fmt"
	"os/exec"
	"text/template"
)

func CreateMachineControllerPod(token string) error {
	tmpl, err := template.ParseFiles("cloud/google/pods/machine-controller.yaml")
	if err != nil {
		return err
	}

	type params struct {
		Token string
	}

	var tmplBuf bytes.Buffer

	err = tmpl.Execute(&tmplBuf, params{
		Token: token,
	})
	if err != nil {
		return err
	}

	cmd := exec.Command("kubectl", "create", "-f", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}

	go func() {
		defer stdin.Close()
		stdin.Write(tmplBuf.Bytes())
	}()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("couldn't create machine controller pod: %v, output: %v", err, out)
	}
	return nil
}
