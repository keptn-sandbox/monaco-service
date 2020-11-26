package common

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/keptn-contrib/dynatrace-sli-service/pkg/common"
	keptnmodels "github.com/keptn/go-utils/pkg/api/models"
	keptnapi "github.com/keptn/go-utils/pkg/api/utils"
	keptnlib "github.com/keptn/go-utils/pkg/lib"
	keptn "github.com/keptn/go-utils/pkg/lib/keptn"
	"gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var RunLocal = (os.Getenv("ENV") == "local")
var RunLocalTest = (os.Getenv("ENV") == "localtest")

/**
 * Defines the Dynatrace Configuration File structure and supporting Constants
 */
const MonacoConfigFilename = "dynatrace/monaco.conf.yaml"
const MonacoConfigFilenameLOCAL = "dynatrace/_monaco.conf.yaml"
const MonacoBaseFolder = "/tmp/monaco/"
const MonacoExecutable = "./monaco"

type MonacoConfigFile struct {
	SpecVersion string   `json:"spec_version" yaml:"spec_version"`
	DtCreds     string   `json:"dtCreds,omitempty" yaml:"dtCreds,omitempty"`
	Projects    []string `json:"projects,omitempty" yaml:"projects,omitempty"`
}

type DTCredentials struct {
	Tenant   string `json:"DT_TENANT" yaml:"DT_TENANT"`
	ApiToken string `json:"DT_API_TOKEN" yaml:"DT_API_TOKEN"`
}

type BaseKeptnEvent struct {
	Context string
	Source  string
	Event   string

	Project            string
	Stage              string
	Service            string
	Deployment         string
	TestStrategy       string
	DeploymentStrategy string

	Image string
	Tag   string

	Labels map[string]string
}

var namespace = getPodNamespace()

func getPodNamespace() string {
	ns := os.Getenv("POD_NAMESPACE")
	if ns == "" {
		return "keptn"
	}

	return ns
}

func GetKubernetesClient() (*kubernetes.Clientset, error) {
	if RunLocal || RunLocalTest {
		return nil, nil
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

//
// replaces $ placeholders with actual values
// $CONTEXT, $EVENT, $SOURCE
// $PROJECT, $STAGE, $SERVICE, $DEPLOYMENT
// $TESTSTRATEGY
// $LABEL.XXXX  -> will replace that with a label called XXXX
// $ENV.XXXX    -> will replace that with an env variable called XXXX
// $SECRET.YYYY -> will replace that with the k8s secret called YYYY
//
func ReplaceKeptnPlaceholders(input string, keptnEvent *BaseKeptnEvent) string {
	result := input

	// FIXING on 27.5.2020: URL Escaping of parameters as described in https://github.com/keptn-contrib/dynatrace-sli-service/issues/54

	// first we do the regular keptn values
	result = strings.Replace(result, "$CONTEXT", url.QueryEscape(keptnEvent.Context), -1)
	result = strings.Replace(result, "$EVENT", url.QueryEscape(keptnEvent.Event), -1)
	result = strings.Replace(result, "$SOURCE", url.QueryEscape(keptnEvent.Source), -1)
	result = strings.Replace(result, "$PROJECT", url.QueryEscape(keptnEvent.Project), -1)
	result = strings.Replace(result, "$STAGE", url.QueryEscape(keptnEvent.Stage), -1)
	result = strings.Replace(result, "$SERVICE", url.QueryEscape(keptnEvent.Service), -1)
	result = strings.Replace(result, "$DEPLOYMENT", url.QueryEscape(keptnEvent.Deployment), -1)
	result = strings.Replace(result, "$TESTSTRATEGY", url.QueryEscape(keptnEvent.TestStrategy), -1)

	// now we do the labels
	for key, value := range keptnEvent.Labels {
		result = strings.Replace(result, "$LABEL."+key, url.QueryEscape(value), -1)
	}

	// now we do all environment variables
	for _, env := range os.Environ() {
		pair := strings.SplitN(env, "=", 2)
		result = strings.Replace(result, "$ENV."+pair[0], url.QueryEscape(pair[1]), -1)
	}

	// TODO: iterate through k8s secrets!

	return result
}

//
// Downloads a resource from the Keptn Configuration Repo
// In RunLocal mode it gets it from the local disk
// In normal mode it first tries to find it on service level, then stage and then project level
//
func GetKeptnResource(keptnEvent *BaseKeptnEvent, resourceURI string, logger *keptn.Logger) (string, error) {

	// if we run in a runlocal mode we are just getting the file from the local disk
	var fileContent string
	if RunLocal {
		localFileContent, err := ioutil.ReadFile(resourceURI)
		if err != nil {
			logMessage := fmt.Sprintf("No %s file found LOCALLY for service %s in stage %s in project %s", resourceURI, keptnEvent.Service, keptnEvent.Stage, keptnEvent.Project)
			logger.Info(logMessage)
			return "", nil
		}
		logger.Info("Loaded LOCAL file " + resourceURI)
		fileContent = string(localFileContent)
	} else {
		resourceHandler := keptnapi.NewResourceHandler(GetConfigurationServiceURL())

		// Lets search on SERVICE-LEVEL
		keptnResourceContent, err := resourceHandler.GetServiceResource(keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, resourceURI)
		if err != nil || keptnResourceContent == nil || keptnResourceContent.ResourceContent == "" {
			// Lets search on STAGE-LEVEL
			keptnResourceContent, err = resourceHandler.GetStageResource(keptnEvent.Project, keptnEvent.Stage, resourceURI)
			if err != nil || keptnResourceContent == nil || keptnResourceContent.ResourceContent == "" {
				// Lets search on PROJECT-LEVEL
				keptnResourceContent, err = resourceHandler.GetProjectResource(keptnEvent.Project, resourceURI)
				if err != nil || keptnResourceContent == nil || keptnResourceContent.ResourceContent == "" {
					// logger.Debug(fmt.Sprintf("No Keptn Resource found: %s/%s/%s/%s - %s", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, resourceURI, err))
					return "", err
				}

				logger.Debug("Found " + resourceURI + " on project level")
			} else {
				logger.Debug("Found " + resourceURI + " on stage level")
			}
		} else {
			logger.Debug("Found " + resourceURI + " on service level")
		}
		fileContent = keptnResourceContent.ResourceContent
	}

	return fileContent, nil
}

// GetMonacoConfig loads monaco.conf for the current service
func GetMonacoConfig(keptnEvent *BaseKeptnEvent, logger *keptn.Logger) (*MonacoConfigFile, error) {

	monacoConfFileContent, err := GetKeptnResource(keptnEvent, MonacoConfigFilename, logger)
	if err != nil {
		return nil, err
	}

	if monacoConfFileContent == "" {
		// loaded an empty file
		logger.Debug("Content of monaco.conf.yaml is empty!")
		return nil, nil
	}

	// unmarshal the file
	monacoConfFile, err := parseMonacoConfigFile([]byte(monacoConfFileContent))

	if err != nil {
		logMessage := fmt.Sprintf("Couldn't parse %s file found for service %s in stage %s in project %s. Error: %s; Content: %s", MonacoConfigFilename, keptnEvent.Service, keptnEvent.Stage, keptnEvent.Project, err.Error(), monacoConfFileContent)
		logger.Error(logMessage)
		return nil, errors.New(logMessage)
	}
	fmt.Printf("GetMonacoConfig monacoConfFile: %v\n", monacoConfFile)
	return monacoConfFile, nil
}

// UploadKeptnResource uploads a file to the Keptn Configuration Service
func UploadKeptnResource(contentToUpload []byte, remoteResourceURI string, keptnEvent *BaseKeptnEvent, logger *keptn.Logger) error {

	// if we run in a runlocal mode we are just getting the file from the local disk
	if RunLocal || RunLocalTest {
		err := ioutil.WriteFile(remoteResourceURI, contentToUpload, 0644)
		if err != nil {
			return fmt.Errorf("Couldnt write local file %s: %v", remoteResourceURI, err)
		}
		logger.Info("Local file written " + remoteResourceURI)
	} else {
		resourceHandler := keptnapi.NewResourceHandler(GetConfigurationServiceURL())

		// lets upload it
		resources := []*keptnmodels.Resource{{ResourceContent: string(contentToUpload), ResourceURI: &remoteResourceURI}}
		_, err := resourceHandler.CreateResources(keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, resources)
		if err != nil {
			return fmt.Errorf("Couldnt upload remote resource %s: %s", remoteResourceURI, *err.Message)
		}

		logger.Info(fmt.Sprintf("Uploaded file %s", remoteResourceURI))
	}

	return nil
}

/**
 * parses the dynatrace.conf.yaml file that is passed as parameter
 */
func parseMonacoConfigFile(input []byte) (*MonacoConfigFile, error) {
	monacoConfFile := &MonacoConfigFile{}
	err := yaml.Unmarshal([]byte(input), &monacoConfFile)

	if err != nil {
		fmt.Printf("Error while parsing: %s\n", err)
		return nil, err
	}
	return monacoConfFile, nil
}

/**
 * Pulls the Dynatrace Credentials from the passed secret
 */
func GetDTCredentials(dynatraceSecretName string) (*DTCredentials, error) {
	if dynatraceSecretName == "" {
		return nil, nil
	}
	dtCreds := &DTCredentials{}
	if RunLocal || RunLocalTest {
		// if we RunLocal we take it from the env-variables
		dtCreds.Tenant = os.Getenv("DT_TENANT")
		dtCreds.ApiToken = os.Getenv("DT_API_TOKEN")
	} else {
		kubeAPI, err := GetKubernetesClient()
		if err != nil {
			return nil, fmt.Errorf("error retrieving Dynatrace credentials: could not initialize Kubernetes client: %v", err)
		}
		secret, err := kubeAPI.CoreV1().Secrets(namespace).Get(dynatraceSecretName, metav1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("error retrieving Dynatrace credentials: could not retrieve secret %s: %v", dynatraceSecretName, err)
		}

		// grabnerandi: remove check on DT_PAAS_TOKEN as it is not relevant for quality-gate-only use case
		if string(secret.Data["DT_TENANT"]) == "" || string(secret.Data["DT_API_TOKEN"]) == "" { //|| string(secret.Data["DT_PAAS_TOKEN"]) == "" {
			return nil, errors.New("invalid or no Dynatrace credentials found. Need DT_TENANT & DT_API_TOKEN stored in secret!")
		}

		dtCreds.Tenant = string(secret.Data["DT_TENANT"])
		dtCreds.ApiToken = string(secret.Data["DT_API_TOKEN"])
	}

	// ensure URL always has http or https in front
	if strings.HasPrefix(dtCreds.Tenant, "https://") || strings.HasPrefix(dtCreds.Tenant, "http://") {
		dtCreds.Tenant = dtCreds.Tenant
	} else {
		dtCreds.Tenant = "https://" + dtCreds.Tenant
	}
	return dtCreds, nil
}

// ParseUnixTimestamp parses a time stamp into Unix foramt
func ParseUnixTimestamp(timestamp string) (time.Time, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsedTime, nil
	}

	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Now(), err
	}
	unix := time.Unix(timestampInt, 0)
	return unix, nil
}

// TimestampToString converts time stamp into string
func TimestampToString(time time.Time) string {
	return strconv.FormatInt(time.Unix()*1000, 10)
}

// Request URL of configuration service
func GetConfigurationServiceURL() string {
	if os.Getenv("CONFIGURATION_SERVICE") != "" {
		return os.Getenv("CONFIGURATION_SERVICE")
	}
	return "configuration-service:8080"
}

// Create base folder for all monaco executions
func CreateBaseFolderIfNotExist() error {
	path := MonacoBaseFolder
	if _, err := os.Stat(path); os.IsNotExist(err) {
		errmkdir := os.Mkdir(path, 0755)
		if errmkdir != nil {
			return errmkdir
		}
	}

	return nil
}

// Create temp folder for keptn context to store project files
func CreateTempFolderForKeptnContext(keptnContext string) error {
	path := MonacoBaseFolder + keptnContext
	if _, err := os.Stat(path); os.IsNotExist(err) {
		errmkdir := os.Mkdir(path, 0755)
		if errmkdir != nil {
			return errmkdir
		}
	}
	return nil
}

// Delete temp folder for cleanup
func DeleteTempFolderForKeptnContext(keptnContext string) error {
	path := MonacoBaseFolder + keptnContext
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		err := os.RemoveAll(path)
		if err != nil {
			fmt.Printf("Error deleting: %s", err)
			return err
		}
	}
	return nil
}

// Copy file contents to a destination
func CopyFileContentToDestination(fileContent string, destination string) error {
	err := ioutil.WriteFile(destination, []byte(fileContent), 0755)

	return err
}

func CopyFileContentsToMonacoProject(fileContent string, keptnContext string) error {
	path := MonacoBaseFolder + keptnContext + "/monaco.zip"
	err := CopyFileContentToDestination(fileContent, path)
	if err != nil {
		return err
	}
	fmt.Println("Succesfully copied to " + path + "\n")
	return err
}

func ExtractMonacoArchive(keptnContext string) error {
	folder := MonacoBaseFolder + keptnContext
	file := folder + "/monaco.zip"
	err := ExtractZIPArchive(file, folder)
	if err != nil {
		return err
	}
	fmt.Println("Succesfully extracted " + file + " to " + folder + "\n")
	return err
}

func ExtractZIPArchive(archiveFileName string, outputFolder string) error {
	files, err := Unzip(archiveFileName, outputFolder)
	if err != nil {
		fmt.Errorf("Error unzipping file: " + err.Error())
		return err
	}
	fmt.Println("Succesfully Unzipped:\n" + strings.Join(files, "\n"))
	return nil
}

func ExecuteMonaco(dtCredentials *DTCredentials, keptnContext string, data *keptnlib.ConfigurationChangeEventData, projects string, verbose bool, dryrun bool) error {

	cmd := exec.Command(MonacoExecutable)

	monacoFolder := MonacoBaseFolder + keptnContext
	// If running in a locla environment, use a local test folder
	if common.RunLocal {
		monacoFolder = "monaco-test"
	}

	if verbose {
		cmd.Args = append(cmd.Args, "-v")
	}
	if dryrun {
		cmd.Args = append(cmd.Args, "-d")
	}
	cmd.Args = append(cmd.Args, "-e=/environments.yaml")
	if projects != "" {
		cmd.Args = append(cmd.Args, "-p="+projects)
	}
	cmd.Args = append(cmd.Args, monacoFolder+"/projects")

	// Set environment variables to be used in monaco
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "DT_ENVIRONMENT_URL="+dtCredentials.Tenant)
	cmd.Env = append(cmd.Env, "DT_API_TOKEN="+dtCredentials.ApiToken)
	cmd.Env = append(cmd.Env, "KEPTN_PROJECT="+data.Project)
	cmd.Env = append(cmd.Env, "KEPTN_SERVICE="+data.Service)
	cmd.Env = append(cmd.Env, "KEPTN_STAGE="+data.Stage)
	fmt.Printf("Monaco command: %v\n", cmd.String())
	stdoutStderr, err := cmd.CombinedOutput()
	fmt.Printf("%s\n", stdoutStderr)

	return err
}

func PrepareFiles(keptnEvent *BaseKeptnEvent, shkeptncontext string, logger *keptn.Logger) error {

	// create base folder
	err := CreateBaseFolderIfNotExist()
	if err != nil {
		logger.Error(fmt.Sprintf("Error creating monaco base folder: %s, breaking", err.Error()))
		return err
	}
	logger.Info(fmt.Sprintf("Monaco base folder created"))

	/// create keptn context folder for project
	err = CreateTempFolderForKeptnContext(shkeptncontext)
	if err != nil {
		logger.Error(fmt.Sprintf("Error creating monaco temp folder for keptncontext %s: %s, breaking", shkeptncontext, err.Error()))
		return err
	}
	logger.Info(fmt.Sprintf("Monaco temp folder created for keptncontext %s", shkeptncontext))

	// Get archive from Keptn
	monacoArchive, err := GetKeptnResource(keptnEvent, "dynatrace/monaco.zip", logger)
	if err != nil {
		logger.Error(fmt.Sprintf("No monaco archive found for project=%s,stage=%s,service=%s found as no dynatrace/monaco.zip in repo: %s, breaking", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, err.Error()))
		return err
	}

	// copy archive
	err = CopyFileContentsToMonacoProject(monacoArchive, shkeptncontext)
	if err != nil {
		logger.Error(fmt.Sprintf("Error copying monaco archive for project=%s,stage=%s,service=%s found as no dynatrace/monaco.zip in repo: %s", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, err.Error()))
		return err
	}
	logger.Info(fmt.Sprintf("Succesfully copied archive for project=%s,stage=%s,service=%s to temp folder", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service))

	// extract archive and copy to folder
	err = ExtractMonacoArchive(shkeptncontext)
	if err != nil {
		logger.Error(fmt.Sprintf("Error extracting archive for project=%s,stage=%s,service=%s : %s, breaking ", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, err.Error()))
		return err
	}
	logger.Info(fmt.Sprintf("Succesfully copied archive for project=%s,stage=%s,service=%s to temp folder %s", keptnEvent.Project, keptnEvent.Stage, keptnEvent.Service, shkeptncontext))

	return nil
}

func GenerateMonacoProjectStringFromMonacoConfig(monacoConfigFile *MonacoConfigFile, keptnEvent *BaseKeptnEvent) string {
	monacoProjectFromConfig := monacoConfigFile.Projects
	monacoProjectString := ""
	if len(monacoProjectFromConfig) == 0 {
		monacoProjectString = keptnEvent.Project
	} else {
		for i, s := range monacoProjectFromConfig {
			monacoProjectString += s
			if i != len(monacoProjectFromConfig)-1 {
				monacoProjectString += ", "
			}
		}
	}
	return monacoProjectString
}

func Unzip(src string, dest string) ([]string, error) {

	var filenames []string

	r, err := zip.OpenReader(src)
	if err != nil {
		return filenames, err
	}
	defer r.Close()

	for _, f := range r.File {

		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name)

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return filenames, fmt.Errorf("%s: illegal file path", fpath)
		}

		filenames = append(filenames, fpath)

		if f.FileInfo().IsDir() {
			// Make Folder
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return filenames, err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return filenames, err
		}

		rc, err := f.Open()
		if err != nil {
			return filenames, err
		}

		_, err = io.Copy(outFile, rc)

		// Close the file without defer to close before next iteration of loop
		outFile.Close()
		rc.Close()

		if err != nil {
			return filenames, err
		}
	}
	return filenames, nil
}
