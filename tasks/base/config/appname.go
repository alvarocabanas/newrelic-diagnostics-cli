package config

import (
	"fmt"
	"strings"

	"github.com/newrelic/newrelic-diagnostics-cli/logger"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
)

var appNameEnvVarKey = "NEW_RELIC_APP_NAME" //PHP does not use env vars
var appNameSysProp = "-Dnewrelic.config.app_name"
var appNameConfigKeys = []string{
	"app_name",         // Java, Node, Python, Ruby
	"newrelic.appname", // PHP
	"AppName",          // GoLang
	"NewRelic.AppName", // .Net for app.config and web.config
	"name",             // .Net for newrelic.config
}

/* Sample for newrelic.config
	<appSettings>
    <add key="NewRelic.AppName" value="App Name" />
	</appSettings>
*/

/* Sample for app.config and web.config
	<application>
    <name> App Name </name>
	</application>
*/

var defaultAppNames = []string{
	"PHP Application",                  // PHP
	"Python Application",               // Python
	"Python Application (Development)", // Python
	"Python Application (Staging)",     // Python
	"My Application",                   // Ruby, .Net, Node, Java
	"My Application (Development)",     // Ruby, .Net, Node, Java
	"My Application (Test)",            // Ruby, .Net, Node, Java
	"My Application (Staging)",         // Ruby, .Net, Node, Java
}

// BaseConfigAppName - Struct for task definition
type BaseConfigAppName struct {
}

// AppNameInfo - Struct to store relevant AppName info
type AppNameInfo struct {
	Name     string
	FilePath string
}

// Identifier - This returns the Category, Subcategory and Name of each task
func (t BaseConfigAppName) Identifier() tasks.Identifier {
	return tasks.IdentifierFromString("Base/Config/AppName")
}

// Explain - Returns the help text for each individual task
func (t BaseConfigAppName) Explain() string {
	return "Check for default application names in New Relic agent configuration."
}

// Dependencies - Returns the dependencies for each task.
func (t BaseConfigAppName) Dependencies() []string {
	return []string{
		"Base/Config/Validate",
		"Base/Env/CollectEnvVars",
		"Base/Env/CollectSysProps",
	}
}

// Execute - The core work within each task
func (t BaseConfigAppName) Execute(options tasks.Options, upstream map[string]tasks.Result) tasks.Result {

	appNameInfoFromEnvVar := getAppNameFromEnvVar(upstream)
	//We can have an early exit because this env var will overwrite all config files setting for app name, except for Python
	if len(appNameInfoFromEnvVar.Name) > 0 {
		return tasks.Result{
			Status:  tasks.Success,
			Summary: fmt.Sprintf("A unique application name was found through the New Relic App name environment variable: %s", appNameInfoFromEnvVar.Name),
			Payload: []AppNameInfo{appNameInfoFromEnvVar}, //though is a single item, we still add them to a slice of AppNameInfo to stay consistent with a future upstream payload type assertion
		}
	}

	//check system properties which takes precedence over config files for Java agent

	appname := getAppNameFromSysProps(upstream)
	if appname != "" {
		return tasks.Result{
			Status:  tasks.Success,
			Summary: fmt.Sprintf("An application name was found through a New Relic system property: %s", appname),
			Payload: []AppNameInfo{{Name: appname, FilePath: appNameSysProp}},
		}
	}

	// No system props then let's check for config files
	if !upstream["Base/Config/Validate"].HasPayload() {
		return tasks.Result{
			Status:  tasks.None,
			Summary: "Task did not meet requirements necessary to run: no validated config files to check",
		}
	}

	configElements, ok := upstream["Base/Config/Validate"].Payload.([]ValidateElement)

	if !ok {
		return tasks.Result{
			Status:  tasks.Error,
			Summary: tasks.AssertionErrorSummary,
		}
	}

	appNameInfosFromConfig := getAppNamesFromConfig(configElements)

	if len(appNameInfosFromConfig) == 0 {
		return tasks.Result{
			Status:  tasks.Warning,
			Summary: "No New Relic app names were found. Please ensure an app name is set in your New Relic agent configuration file or as a New Relic environment variable (NEW_RELIC_APP_NAME). Ignore this warning if you are troubleshooting for a non APM Agent.",
			URL:     "https://docs.newrelic.com/docs/agents/manage-apm-agents/app-naming/name-your-application",
		}
	}

	defaultNameMatches := ""
	for _, appNameInfo := range appNameInfosFromConfig {
		for _, defaultName := range defaultAppNames {
			if appNameInfo.Name == defaultName {
				defaultNameMatches += fmt.Sprintf("\n\t\"%s\" as specified in %s", appNameInfo.Name, appNameInfo.FilePath)
			}

		}
	}

	var defaultWarning = "\nMultiple applications with the same default appname will all report to the same source. Consider changing to a unique appname and review the recommended documentation"
	if len(defaultNameMatches) > 0 {
		return tasks.Result{
			Status:  tasks.Warning,
			Summary: fmt.Sprintf("One or more of your applications is using a default appname: %s %s", defaultNameMatches, defaultWarning),
			URL:     "https://docs.newrelic.com/docs/agents/manage-apm-agents/app-naming/name-your-application",
		}
	}

	return tasks.Result{
		Status:  tasks.Success,
		Summary: fmt.Sprintf("%d unique application name(s) found: %s", len(appNameInfosFromConfig), appNameInfosFromConfig[0].Name),
		Payload: appNameInfosFromConfig,
	}
}

func getAppNameFromEnvVar(upstream map[string]tasks.Result) AppNameInfo {

	if upstream["Base/Env/CollectEnvVars"].Status == tasks.Info {
		envVars, ok := upstream["Base/Env/CollectEnvVars"].Payload.(map[string]string)

		if !ok {
			logger.Debug("Task did not meet requirements necessary to run: type assertion failure")
		}
		appname, isPresent := envVars[appNameEnvVarKey]
		if !isPresent {
			return AppNameInfo{}
		}
		return AppNameInfo{
			Name:     appname,
			FilePath: appNameEnvVarKey,
		}
	}
	return AppNameInfo{}
}

func getAppNamesFromConfig(configElements []ValidateElement) []AppNameInfo {

	result := []AppNameInfo{}

	for _, configFile := range configElements {
		primaryNameKey := findPrimaryNameKeyInConfigFile(configFile)
		configFilePath := configFile.Config.FilePath
		configFileName := configFile.Config.FileName
		/*
			Only grab the first appname key found as this is the main required for an app to start reporting. The other appname keys are optional. Example:
				/common/app_name: Luces-sqs-java
				/development/app_name: My Application (Development)
				/production/app_name: My Application (Production)
				/staging/app_name: My Application (Staging)
				/test/app_name: My Application (Test)
		*/
		if len(primaryNameKey.Key) > 0 {
			if !primaryNameKey.IsLeaf() {
				for _, child := range primaryNameKey.Children {
					appName := child.Value()
					result = append(result, AppNameInfo{
						Name:     appName,
						FilePath: fmt.Sprintf("%s%s", configFilePath, configFileName),
					})
				}
			} else {
				appName := primaryNameKey.Value()

				if len(appName) > 0 {
					result = append(result, AppNameInfo{
						Name:     appName,
						FilePath: fmt.Sprintf("%s%s", configFilePath, configFileName),
					})
				}
			}
		}
	}
	return result
}

func findPrimaryNameKeyInConfigFile(configFile ValidateElement) tasks.ValidateBlob {

	for i := 0; i < len(appNameConfigKeys); i++ {
		foundKeys := configFile.ParsedResult.FindKey(appNameConfigKeys[i])
		if len(foundKeys) == 1 {
			return foundKeys[0]
		} else if len(foundKeys) > 1 {
			for _, validateBlob := range foundKeys {
				if strings.Contains(validateBlob.Path, "common") {
					return validateBlob
				}
			}
			return foundKeys[0] //backup plan because it seems predetermined that the primary name shows up first
		}

	}
	return tasks.ValidateBlob{}
}

func getAppNameFromSysProps(upstream map[string]tasks.Result) string {
	if upstream["Base/Env/CollectSysProps"].Status != tasks.Info {
		return ""
	}

	sysProps, ok := upstream["Base/Env/CollectSysProps"].Payload.([]tasks.ProcIDSysProps)

	if !ok {
		logger.Debug("Task did not meet requirements necessary to run: type assertion failure")
		return ""
	}

	for i := 0; i < len(sysProps); i++ {
		sysPropMap := sysProps[i].SysPropsKeyToVal
		appName, isPresent := sysPropMap[appNameSysProp]
		if isPresent {
			return appName
		}
	}
	return ""
}
