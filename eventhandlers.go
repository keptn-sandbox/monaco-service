package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2" // make sure to use v2 cloudevents here
	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"

	"github.com/keptn-sandbox/monaco-service/pkg/common"
)

/**
* Here are all the handler functions for the individual event
* See https://github.com/keptn/spec/blob/0.8.0-alpha/cloudevents.md for details on the payload
**/

// GenericLogKeptnCloudEventHandler is a generic handler for Keptn Cloud Events that logs the CloudEvent
func GenericLogKeptnCloudEventHandler(myKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data interface{}) error {
	log.Printf("Handling %s Event: %s", incomingEvent.Type(), incomingEvent.Context.GetID())
	log.Printf("CloudEvent %T: %v", data, data)

	return nil
}

// HandleConfigureMonitoringTriggeredEvent handles configure-monitoring.triggered events
// TODO: add in your handler code
func HandleConfigureMonitoringTriggeredEvent(myKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data *keptnv2.ConfigureMonitoringTriggeredEventData) error {
	log.Printf("Handling configure-monitoring.triggered Event: %s", incomingEvent.Context.GetID())

	return nil
}

// HandleConfigureMonitoringTriggeredEvent handles configure-monitoring.triggered events
// TODO: add in your handler code
func HandleMonacoTriggeredEvent(myKeptn *keptnv2.Keptn, incomingEvent cloudevents.Event, data *keptnv2.EventData) error {
	log.Printf("Handling monaco.triggered Event: %s", incomingEvent.Context.GetID())

	_, err := myKeptn.SendTaskStartedEvent(data, ServiceName)

	var shkeptncontext string
	incomingEvent.Context.ExtensionAs("shkeptncontext", &shkeptncontext)

	log.Println(fmt.Sprintf("Handling Configuration Changed Event: %s", incomingEvent.Context.GetID()))
	log.Println(fmt.Sprintf("Processing sh.keptn.event.configuration.change for %s.%s.%s", data.Project, data.Stage, data.Service))

	keptnEvent := &common.BaseKeptnEvent{}
	keptnEvent.Project = data.Project
	keptnEvent.Stage = data.Stage
	keptnEvent.Service = data.Service
	keptnEvent.Labels = data.Labels
	keptnEvent.Context = shkeptncontext

	monacoConfigFile, _ := common.GetMonacoConfig(keptnEvent)
	dtCreds := ""
	if monacoConfigFile != nil {
		// implementing https://github.com/keptn-contrib/dynatrace-sli-service/issues/90
		dtCreds = common.ReplaceKeptnPlaceholders(monacoConfigFile.DtCreds, keptnEvent)
		log.Println("Found monaco.conf.yaml with DTCreds: " + dtCreds)
	} else {
		log.Println("Using default DTCreds: dynatrace as no custom monaco.conf.yaml was found!")
		monacoConfigFile = &common.MonacoConfigFile{}
		monacoConfigFile.DtCreds = "dynatrace"
	}

	//
	// Adding DtCreds as a label so users know which DtCreds was used
	if data.Labels == nil {
		data.Labels = make(map[string]string)
	}
	data.Labels["DtCreds"] = monacoConfigFile.DtCreds

	dtCredentials, err := getDynatraceCredentials(dtCreds, data.Project)

	if err != nil {
		log.Fatal("Failed to fetch Dynatrace credentials: " + err.Error())
		return err
	}

	// Prepare the folder structure for monaco (create base + shkeptncontext temp folder, copy files, get monaco.zip, extract and copy to temp)
	err = common.PrepareFiles(keptnEvent)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error preparing monaco files: %s", err.Error()))
		return err
	}

	// generate projects string for monaco
	monacoProjects := common.GenerateMonacoProjectStringFromMonacoConfig(monacoConfigFile, keptnEvent)

	// test and apply monaco configuration
	err = callMonaco(dtCredentials, keptnEvent, monacoProjects)

	keeptempString := os.Getenv("MONACO_KEEP_TEMP_DIR")
	if keeptempString == "" {
		keeptempString = "true"
	}
	keeptemp, _ := strconv.ParseBool(keeptempString)

	if keeptemp {
		log.Println(fmt.Sprintf("Not deleting temp folder (MONACO_KEEP_TEMP_DIR=true) for %s", keptnEvent.Context))
	} else {
		// Clean up: remove temp folder for Context
		err = common.DeleteTempFolderForKeptnContext(keptnEvent)
		log.Println(fmt.Sprintf("Delete temp folder for %s", keptnEvent.Context))
	}

	_, err = myKeptn.SendTaskFinishedEvent(data, ServiceName)

	return nil
}

func getDynatraceCredentials(secretName string, project string) (*common.DTCredentials, error) {

	secretNames := []string{secretName, fmt.Sprintf("dynatrace-credentials-%s", project), "dynatrace-credentials", "dynatrace"}

	for _, secret := range secretNames {
		if secret == "" {
			continue
		}

		dtCredentials, err := common.GetDTCredentials(secret)

		if err != nil {
			log.Fatal(fmt.Sprintf("Error retrieving secret '%s': %v", secret, err))
		}

		if err == nil && dtCredentials != nil {
			// lets validate if the tenant URL is
			log.Println(fmt.Sprintf("Secret '%s' with credentials found, returning (%s) ...", secret, dtCredentials.Tenant))
			return dtCredentials, nil
		}
	}

	return nil, errors.New("Could not find any Dynatrace specific secrets with the following names: " + strings.Join(secretNames, ","))
}

func callMonaco(dtCredentials *common.DTCredentials, keptnEvent *common.BaseKeptnEvent, projects string) error {

	// Get Env-Variables on whether we should first do a dry run and whether we should do verbose
	verboseString := os.Getenv("MONACO_VERBOSE_MODE")
	if verboseString == "" {
		verboseString = "true"
	}
	dryrunString := os.Getenv("MONACO_DRYRUN")
	if dryrunString == "" {
		dryrunString = "true"
	}

	verbose, _ := strconv.ParseBool(verboseString)
	dryrun, _ := strconv.ParseBool(dryrunString)

	if dryrun {
		// Dry Run to test configuration structure
		err := common.ExecuteMonaco(dtCredentials, keptnEvent, projects, verbose, true)
		if err != nil {
			return err
		}
	}

	// Apply configuration
	err := common.ExecuteMonaco(dtCredentials, keptnEvent, projects, verbose, false)

	return err
}
