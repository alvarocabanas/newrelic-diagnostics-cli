package output

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"

	"github.com/newrelic/newrelic-diagnostics-cli/config"
	log "github.com/newrelic/newrelic-diagnostics-cli/logger"
	"github.com/newrelic/newrelic-diagnostics-cli/output/color"
	"github.com/newrelic/newrelic-diagnostics-cli/registration"
	"github.com/newrelic/newrelic-diagnostics-cli/scriptrunner"
	"github.com/newrelic/newrelic-diagnostics-cli/tasks"
)

// WriteOutputHeader takes in array of Result structs, returns color coded results overview in following format: <taskIdentifier>:<result>
func WriteOutputHeader() {
	log.Info(color.ColorString(color.White, "\nCheck Results\n-------------------------------------------------"))
}

// WriteSummary reports on any non-successful items and tells the user why they weren't successful
func WriteSummary(data []registration.TaskResult) {
	if len(data) < 1 {
		return
	}
	var failures []registration.TaskResult
	for _, result := range data {
		if result.Result.IsFailure() {
			failures = append(failures, result)
		}
	}

	if len(failures) == 0 {
		log.Info(color.ColorString(color.White, "\nNo Issues Found\n"))
	} else {
		log.Info(color.ColorString(color.White, "\nIssues Found\n-------------------------------------------------"))

	}

	filteredCounter := 0
	var filtered [6]int //Int array corresponding with 6 statuses, to count any filtered results

	for _, result := range failures {
		if filteredResult(result.Result.StatusToString()) {
			log.Info(color.ColorString(result.Result.Status, result.Result.StatusToString()), "-", result.Task.Identifier().String())
			log.Info(result.Result.Summary)
			if result.Result.URL != "" {
				log.Info("See " + result.Result.URL + " for more information.")
			}
			log.Infof("\n")
		} else {
			filteredCounter++
			filtered[result.Result.Status]++
		}
	}

	//If -filter caused some results to be filtered, provide a summary of filtered results
	if filteredCounter > 0 {
		filteredOutput := color.ColorString(color.Gray, "\n"+strconv.Itoa(filteredCounter)+" issues not shown: "+filteredToString(filtered)+"\n(Use \"-filter all\" to see all issues)")
		log.Info(filteredOutput)
	}
}

func PrintScriptOutput(data string) {
	if !config.Flags.Quiet {
		log.Info(color.ColorString(color.White, "\nScript Output\n--------------------------------------------------"))
	}
	log.Info(data)
}

func WriteScriptOutputFile(filename string, output []byte, cmdLineOptions tasks.Options) {
	keepGoing := true
	if tasks.FileExists(filename) {
		log.Infof("File already exists: %s\n", filename)
		keepGoing = tasks.PromptUser("Would you like to overwrite it?", cmdLineOptions)
	}
	if keepGoing {
		err := os.WriteFile(filename, output, 0644)
		if err != nil {
			log.Infof("Failed to save script output: %s\n", err.Error())
		}
	}
}

func CopyScriptOutputsToZip(scriptData *scriptrunner.ScriptData, zipfile *zip.Writer) error {
	filelist := []string{scriptData.OutputPath}

	filelist = append(filelist, scriptData.AddtlFiles...)
	for _, filename := range filelist {
		info, err := os.Stat(filename)
		if err != nil {
			return err
		}

		file, err := os.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash("nrdiag-output/ScriptOutput/" + filename)
		header.Method = zip.Deflate
		writer, err := zipfile.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, file)
		if err != nil {
			return err
		}

		addFileToFileList(tasks.FileCopyEnvelope{
			Path:       filename,
			Identifier: "ScriptOutput/",
		})
	}

	return nil
}

// WriteOutputFile will output a JSON file with the results of the run
func WriteOutputFile(data []registration.TaskResult, scriptResults *scriptrunner.ScriptData) {
	outputJSON(getResultsJSON(data, scriptResults))
}

// ProcessFilesChannel - reads from the channels for files to copy and deals with them
func ProcessFilesChannel(zipfile *zip.Writer, wg *sync.WaitGroup) {
	// This is how we track the file names going into to zip file to prevent duplicates
	// map of [string]struct is used because empty struct takes no memory
	fileList := make(map[string]struct{})
	pathList := make(map[string]struct{})
	var taskFiles []tasks.FileCopyEnvelope

	for result := range registration.Work.FilesChannel {
		log.Debug("Copying files from result: ", result.Task.Identifier().String())

		for _, envelope := range result.Result.FilesToCopy {
			log.Debug("Copying file: ", envelope.Path)
			if envelope.Stream == nil && !tasks.FileExists(envelope.Path) {
				log.Debugf("File does not exist, skipping: '%s'\n", envelope.Path)
				continue
			}
			isExecutable, exeErr := envelope.IsExecutable()
			if exeErr != nil {
				log.Debugf("Unable to determine if file is executable, skipping: '%s'\n", envelope.Path)
				continue
			}
			if isExecutable {
				log.Debugf("Skipping executable file: '%s'\n", envelope.Path)
				continue
			}
			// check for duplicate file paths
			if envelope.Stream == nil && mapContains(pathList, envelope.Path) {
				log.Debugf("Already added '%s' to the file list. Skipping.\n", envelope.Path)
			} else {
				for i := 1; i < 50; i++ { //if we can't find a unique name in 50 tries, give up!
					if !mapContains(fileList, envelope.StoreName()) {
						log.Debug("file name is ", envelope.StoreName(), " for ", envelope.Path)
						fileList[envelope.StoreName()] = struct{}{}
						pathList[envelope.Path] = struct{}{}
						// Set the identifier if not previously set
						if envelope.Identifier == "" {
							envelope.Identifier = result.Task.Identifier().String()
						}
						taskFiles = append(taskFiles, envelope)
						break
					} else {
						log.Debug("tried ", envelope.StoreName(), "... keep looking.")
						envelope.IncrementDuplicateCount()
					}
				}
			}
		}

	}
	copyFilesToZip(zipfile, taskFiles)

	log.Debug("Files channel closed")
	log.Debug("Decrementing wait group in processFilesChannel.")
	wg.Done()
}

// CopySingleFileToZip - takes the named file and adds it to the zip file (assumes relative location to OutputPath)
func CopySingleFileToZip(zipfile *zip.Writer, filename string) {
	filePath := filepath.Join(config.Flags.OutputPath, filename)
	_, filelistErr := os.Stat(filePath)
	if os.IsNotExist(filelistErr) {
		log.Debug("Could not copy file to zip: ", filename)
		log.Debug("Error creating filelist was:", filelistErr)
		return
	}

	// Now add the filelist to the zip
	filelist := []tasks.FileCopyEnvelope{
		{Path: filePath},
	}
	copyFilesToZip(zipfile, filelist)
}

// CopyOutputToZip - takes the nrdiag-output.json and adds it to the zip file
func CopyOutputToZip(zipfile *zip.Writer) {
	CopySingleFileToZip(zipfile, "nrdiag-output.json")
}

func CopyFileListToZip(zipfile *zip.Writer) {
	CopySingleFileToZip(zipfile, "nrdiag-filelist.txt")
}

func HandleIncludeFlag(zipfile *zip.Writer, includePath string) {
	if _, err := os.Stat(includePath); err == nil {
		fileSize, err := GetTotalSize(includePath)
		if err != nil {
			log.Debugf("Error getting size: %s", err.Error())
		}
		if fileSize > 3999999999 {
			log.Fatalf("The file(s) that you included were 4GB or larger.  Please specify a smaller file")
		}

		_err := CopyIncludePathToZip(zipfile, includePath)
		if _err != nil {
			log.Debugf("Error adding to zip: %s", _err.Error())
		}

	} else if errors.Is(err, os.ErrNotExist) {
		log.Infof(color.ColorString(color.Yellow, "Error: no files found at: %s\n"), includePath)
	} else {
		log.Info(err)

	}
}

func GetTotalSize(pathToDir string) (int64, error) {
	var totalFileSize int64 = 0
	err := filepath.Walk(pathToDir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			totalFileSize += WalkSizeFunction(info)
			return nil
		})
	return totalFileSize, err
}

func CopyIncludePathToZip(zipfile *zip.Writer, pathToDir string) error {
	err := filepath.Walk(pathToDir,
		func(path string, info os.FileInfo, err error) error {
			ok := WalkCopyFunction(path, info, err, zipfile, WriteFileToZip)
			return ok
		})
	return err

}

// WriteLineResults - outputs results to the screen as they complete (from the channel) and then returns the entire set
func WriteLineResults() []registration.TaskResult {
	filteredCounter := 0
	var filtered [6]int

	var outputResults []registration.TaskResult
	hsmResult := tasks.Result{}
	hsmPayload := make(map[string]bool)

	for result := range registration.Work.ResultsChannel {
		if filteredResult(result.Result.StatusToString()) {
			payload := ""
			if result.Task.Identifier().String() == "Base/Config/ValidateHSM" && result.Result.Payload != nil {
				hsmPayload = result.Result.Payload.(map[string]bool)
				hsmResult = result.Result
			}
			if result.Result.Status == tasks.Info {
				truncated := ""
				newlineRegexp := regexp.MustCompile(`\n`)
				newSummary := newlineRegexp.ReplaceAllString(result.Result.Summary, " ")
				if len(newSummary) > 50 {
					truncated = "..."
				}
				payload = fmt.Sprintf(" [%.50s%s] ", newSummary, truncated)
			}
			if result.WasOverride {
				log.FixedPrefix(20, color.ColorString(result.Result.Status, result.Result.Status.StatusToString()), result.Task.Identifier().String()+payload+color.ColorString(color.LightRed, " (override)"))
			} else {
				log.FixedPrefix(20, color.ColorString(result.Result.Status, result.Result.Status.StatusToString()), result.Task.Identifier().String()+payload)
			}
		} else {
			//Using 2 here because filteredCounter is also used to determine if we've filtered anything to initiate that block later on.
			filteredCounter++
			filtered[result.Result.Status]++
		}
		log.Debug("Done with ", result.Task.Identifier(), " in output results")
		outputResults = append(outputResults, result)
	}

	//If -filter caused some results to be filtered, provide a summary of filtered results
	if filteredCounter > 0 {

		var partialMessage string
		if config.Flags.VeryQuiet {
			partialMessage = " results: "
		} else {
			partialMessage = " results not shown: "
		}

		filteredOutput := color.ColorString(color.Gray, strconv.Itoa(filteredCounter)+partialMessage+filteredToString(filtered))
		log.Info(filteredOutput)
	}
	if len(hsmPayload) > 0 {
		log.Infof(hsmResult.Summary)
		if config.Flags.AutoAttach || config.Flags.APIKey != "" {
			log.Info("See your uploaded results to validate High Security Mode settings.\n")
		} else {
			log.Info("To upload results and validate High Security Mode settings, run the Diagnostics CLI with the -a or -api-key flag.\n")

		}
	}
	if len(outputResults) > 0 {
		log.Info("See nrdiag-output.json for full results.")
	}
	log.Debug("Done with writeLineResults")
	return outputResults
}
