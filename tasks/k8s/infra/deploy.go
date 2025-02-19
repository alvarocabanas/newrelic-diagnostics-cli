package infra

import (
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
)

// K8sDeployment - This struct defined the sample plugin which can be used as a starting point
type K8sDeployment struct {
	cmdExec tasks.CmdExecFunc
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (p K8sDeployment) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString("K8s/Infra/Deploy")
}

// Explain - Returns the help text for each individual task
func (p K8sDeployment) Explain() string {
	return "Collects newrelic-infrastructure deployment information."
}

// Dependencies - Returns the dependencies for each task.
func (p K8sDeployment) Dependencies() []string {
	return []string{}
}

// Execute - The core work within each task
func (p K8sDeployment) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result {
	var (
		res []byte
		err error
	)

	namespace := options.Options["namespace"]
	res, err = p.runCommand(namespace)
	if err != nil {
		return tasks.Result{
			Summary: "Error retrieving deployment details: " + err.Error(),
			Status:  tasks.Error,
		}
	}

	stream := make(chan string)
	go tasks.StreamBlob(string(res), stream)

	return tasks.Result{
		Summary:     "Successfully collected K8s newrelic-infrastructure deployment",
		Status:      tasks.Info,
		FilesToCopy: []tasks.FileCopyEnvelope{{Path: "k8sInfraDeployment.txt", Stream: stream}},
	}
}

func (p K8sDeployment) runCommand(namespace string) ([]byte, error) {
	if namespace == "" {
		return p.cmdExec(
			kubectlBin,
			"describe",
			"deployment",
			"-l",
			"app.kubernetes.io/name=newrelic-infrastructure",
		)
	}
	return p.cmdExec(
		kubectlBin,
		"describe",
		"deployment",
		"-n",
		namespace,
		"-l",
		"app.kubernetes.io/name=newrelic-infrastructure",
	)
}
