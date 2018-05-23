package kubeadm

import (
	"sigs.k8s.io/cluster-api/pkg/cmd-runner"
	"strings"
	"time"
)

type Kubeadm struct {
	runner cmd_runner.CmdRunner
}

// see https://kubernetes.io/docs/reference/setup-tools/kubeadm/kubeadm-token/ for an explanation of the parameters
type TokenCreateParams struct {
	Config           string
	Description      string
	Groups           []string
	Help             bool
	PrintJoinCommand bool
	Ttl              time.Duration
	Usages           []string
}

func NewWithCmdRunner(runner cmd_runner.CmdRunner) *Kubeadm {
	return &Kubeadm{
		runner: runner,
	}
}

func New() *Kubeadm {
	return NewWithCmdRunner(cmd_runner.New())
}

func (k *Kubeadm) TokenCreate(params TokenCreateParams) (string, error) {
	args := []string{"token", "create"}
	args = appendStringParamIfPresent(args, "--config", params.Config)
	args = appendStringParamIfPresent(args, "--description", params.Description)
	args = appendStringSliceIfValid(args, "--groups", params.Groups)
	args = appendFlagIfTrue(args, "--help", params.Help)
	args = appendFlagIfTrue(args, "--print-join-command", params.PrintJoinCommand)
	if params.Ttl != time.Duration(0) {
		args = append(args, "--ttl")
		args = append(args, params.Ttl.String())
	}
	args = appendStringSliceIfValid(args, "--usages", params.Usages)
	return k.runner.CombinedOutput("kubeadm", args...)
}

func appendFlagIfTrue(args []string, paramName string, value bool) []string {
	if value {
		return append(args, paramName)
	}
	return args
}

func appendStringParamIfPresent(args []string, paramName string, value string) []string {
	if value == "" {
		return args
	}
	args = append(args, paramName)
	return append(args, value)
}

func appendStringSliceIfValid(args []string, paramName string, values []string) []string {
	if len(values) == 0 {
		return args
	}
	args = append(args, paramName)
	return append(args, toStringSlice(values))
}

func toStringSlice(args []string) string {
	return strings.Join(args, ":")
}
