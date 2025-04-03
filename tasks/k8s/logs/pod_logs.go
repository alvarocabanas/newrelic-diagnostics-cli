package logs

import (
	"fmt"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
)

// K8sLogs - This struct defined the sample plugin which can be used as a starting point
type K8sPodLogs struct {
	cmdExec       tasks.CmdExecFunc
	appName       string
	labelSelector string
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (p K8sPodLogs) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString(fmt.Sprintf("K8s/Logs/%s", p.appName))
}

// Explain - Returns the help text for each individual task
func (p K8sPodLogs) Explain() string {
	return "Collects  " + p.appName + " pod logs"
}

// Dependencies - Returns the dependencies for each task.
func (p K8sPodLogs) Dependencies() []string {
	return []string{}
}

// Execute - The core work within each task
func (p K8sPodLogs) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result {
	var (
		res []byte
		err error
	)

	namespace := options.Options["k8sNamespace"]
	res, err = p.runCommand(namespace)
	if err != nil {
		return tasks.Result{
			Summary: "Error retrieving logs: " + err.Error(),
			Status:  tasks.Error,
		}
	}

	stream := make(chan string)
	go tasks.StreamBlob(string(res), stream)

	return tasks.Result{
		Summary:     "Successfully collected K8s " + p.appName + " pod logs",
		Status:      tasks.Info,
		FilesToCopy: []tasks.FileCopyEnvelope{{Path: fmt.Sprintf("%s.log", p.appName), Stream: stream}},
	}
}

func (p K8sPodLogs) runCommand(namespace string) ([]byte, error) {
	if namespace == "" {
		return p.cmdExec(
			kubectlBin,
			"logs",
			"-l",
			p.labelSelector,
			"--all-containers",
			"--prefix",
		)
	}
	return p.cmdExec(
		kubectlBin,
		"logs",
		"-n",
		namespace,
		"-l",
		p.labelSelector,
		"--all-containers",
		"--prefix",
	)
}
