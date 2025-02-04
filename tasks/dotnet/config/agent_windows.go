package config

import (
	"strconv"
	"strings"

	log "github.com/newrelic/newrelic-diagnostics-cli/logger"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks/base/config"
)

// DotNetConfigAgent - This struct defined the sample plugin which can be used as a starting point
type DotNetConfigAgent struct { // This defines the task itself and should be named according to the standard CategorySubcategoryTaskname in camelcase
	name string
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (p DotNetConfigAgent) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString("DotNet/Config/Agent") // This should be updated to match the struct name
}

// Explain - Returns the help text for each individual task
func (p DotNetConfigAgent) Explain() string {
	return "Validate New Relic .NET agent configuration file(s)" //This is the customer visible help text that describes what this particular task does
}

// Dependencies - Returns the dependencies for ech task. When executed by name each dependency will be executed as well and the results from that dependency passed in to the downstream task
func (p DotNetConfigAgent) Dependencies() []string {
	return []string{
		"Base/Config/Validate", //This identifies this task as dependent on "Base/Config/Validate" and so the results from that task will be passed to this task. See the execute method to see how to interact with the results.
		"DotNet/Agent/Installed",
	}
}

var newrelicConfigKeys = []string{
	"-agentEnabled",
	"-licenseKey",
}

var webAppConfigKeys = []string{
	"-key",
	"-targetFramework",
}

// Execute - The core work within each task
func (p DotNetConfigAgent) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result { //By default this task is commented out. To see it run go to the tasks/registerTasks.go file and uncomment the w.Register for this task

	// abort if it isn't installed
	if upstream["DotNet/Agent/Installed"].Status != tasks.Success {
		if upstream["DotNet/Agent/Installed"].Summary == tasks.NoAgentDetectedSummary {
			return tasks.Result{
				Status:  tasks.None,
				Summary: tasks.NoAgentUpstreamSummary + "DotNet/Agent/Installed",
			}
		}
		return tasks.Result{
			Status:  tasks.None,
			Summary: tasks.UpstreamFailedSummary + "DotNet/Agent/Installed",
		}
	}

	// get all the config files and elements to check them. No need to verify if this task succeeded because we already checked this on the upstream task DotNet/Agent/Installed
	configFiles, ok := upstream["Base/Config/Validate"].Payload.([]config.ValidateElement) //This is a type assertion to cast my upstream results back into data I know the structure of and can now work with. In this case, I'm casting it back to the []validateElements{} I know it should return
	if !ok {
		return tasks.Result{
			Status:  tasks.Error,
			Summary: tasks.AssertionErrorSummary,
		}
	}
	log.Debug("Successfully gathered config files from upstream.")

	// validate the config files elements
	filesToAdd, err := checkConfigs(configFiles) //err is a boolean
	if err {
		return tasks.Result{
			Status:  tasks.Warning,
			Summary: "Unable to validate the .NET agent config files because the files do not contain typical .NET agent configuration settings.",
		}
	}
	// no error means at least one file validated
	return tasks.Result{
		Status:  tasks.Success,
		Summary: "Found " + strconv.FormatInt(int64(len(filesToAdd)), 10) + " .NET agent config files.",
		Payload: filesToAdd,
	}
}

func checkConfigs(configFiles []config.ValidateElement) ([]config.ValidateElement, bool) {
	var filesValidated []config.ValidateElement
	var keysFound []string
	var keysToCheck []string

	// loop through each config found
	for _, configFile := range configFiles {
		var fullPath = configFile.Config.FilePath + configFile.Config.FileName
		// clear previously found keys
		keysFound = nil

		// check filename, set up keysToCheck variable
		if strings.EqualFold(configFile.Config.FileName, "newrelic.config") {
			keysToCheck = newrelicConfigKeys
		} else if strings.EqualFold(configFile.Config.FileName, "web.config") || tasks.CaseInsensitiveStringContains(configFile.Config.FileName, ".exe.config") {
			keysToCheck = webAppConfigKeys
		} else { // name doesn't match anything, skip it
			log.Debug("Filename does not match newrelic.config, web.config, or *.exe.config pattern. Ignoring file:", fullPath)
			continue
		}

		// loop through keys and check for each
		for _, key := range keysToCheck {
			keyFound := configFile.ParsedResult.FindKey(key)
			if len(keyFound) > 0 {
				log.Debug("Found this key in the config file:", key)
				keysFound = append(keysFound, key)
			} else {
				log.Debug("Could not find this key in the config file:", key)
			}
		}
		if len(keysFound) > 0 {
			log.Debug(len(keysFound), "out of", len(keysToCheck), "keys found")
			log.Debug("Adding file to payload:", fullPath)
			filesValidated = append(filesValidated, configFile)
		} else {
			// no keys were found, lets check the raw file
			log.Debug("No keys found, checking raw file:", fullPath)
			rawCheckOk := checkRawFile(fullPath)
			if rawCheckOk {
				log.Debug("Successfully validated raw file:", fullPath)
				filesValidated = append(filesValidated, configFile)
			} else {
				// no keys or raw strings found, ignore file
				log.Debug("Raw file did not validate, ignoring file:", fullPath)
			}
		}
	}

	//Check for one or more filesValidated
	if len(filesValidated) > 0 {
		log.Debug(len(filesValidated), "out of", len(configFiles), "config files successfully validated.")
		return filesValidated, false
	}

	// If it gets here, no files validated successfully
	log.Debug("No configuration elements found in any files!")
	return filesValidated, true
}

func checkRawFile(path string) bool {
	// for both newrelic.config and web/app.config, can check for "<configuration"
	return tasks.FindStringInFile("[<]configuration[>]?", path)
}
