package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cloudevents/sdk-go/pkg/cloudevents"
	keptn "github.com/keptn/go-utils/pkg/lib"
	keptnlib "github.com/keptn/go-utils/pkg/lib"
	keptnlog "github.com/keptn/go-utils/pkg/lib/keptn"

	"github.com/kristofre/monaco-service/pkg/common"
)

/**
* Here are all the handler functions for the individual event
  See https://github.com/keptn/spec/blob/0.1.3/cloudevents.md for details on the payload

  -> "sh.keptn.event.configuration.change"
  -> "sh.keptn.events.deployment-finished"
  -> "sh.keptn.events.tests-finished"
  -> "sh.keptn.event.start-evaluation"
  -> "sh.keptn.events.evaluation-done"
  -> "sh.keptn.event.problem.open"
	-> "sh.keptn.events.problem"
	-> "sh.keptn.event.action.triggered"
*/

// Handles ConfigureMonitoringEventType = "sh.keptn.event.monitoring.configure"
func HandleConfigureMonitoringEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.ConfigureMonitoringEventData) error {
	log.Printf("Handling Configure Monitoring Event: %s", incomingEvent.Context.GetID())

	return nil
}

//
// Handles ConfigurationChangeEventType = "sh.keptn.event.configuration.change"
// TODO: add in your handler code
//
func HandleConfigurationChangeEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.ConfigurationChangeEventData) error {
	var shkeptncontext string
	incomingEvent.Context.ExtensionAs("shkeptncontext", &shkeptncontext)

	stdLogger := keptnlog.NewLogger(shkeptncontext, incomingEvent.Context.GetID(), "monaco-service")
	stdLogger.Info(fmt.Sprintf("Handling Configuration Changed Event: %s", incomingEvent.Context.GetID()))
	stdLogger.Info(fmt.Sprintf("Processing sh.keptn.event.configuration.change for %s.%s.%s", data.Project, data.Stage, data.Service))

	keptnEvent := &common.BaseKeptnEvent{}
	keptnEvent.Project = data.Project
	keptnEvent.Stage = data.Stage
	keptnEvent.Service = data.Service
	keptnEvent.Labels = data.Labels
	keptnEvent.Context = shkeptncontext

	monacoConfigFile, _ := common.GetMonacoConfig(keptnEvent, stdLogger)
	dtCreds := ""
	if monacoConfigFile != nil {
		// implementing https://github.com/keptn-contrib/dynatrace-sli-service/issues/90
		dtCreds = common.ReplaceKeptnPlaceholders(monacoConfigFile.DtCreds, keptnEvent)
		stdLogger.Debug("Found monaco.conf.yaml with DTCreds: " + dtCreds)
	} else {
		stdLogger.Debug("Using default DTCreds: dynatrace as no custom monaco.conf.yaml was found!")
		monacoConfigFile = &common.MonacoConfigFile{}
		monacoConfigFile.DtCreds = "dynatrace"
	}

	//
	// Adding DtCreds as a label so users know which DtCreds was used
	if data.Labels == nil {
		data.Labels = make(map[string]string)
	}
	data.Labels["DtCreds"] = monacoConfigFile.DtCreds

	dtCredentials, err := getDynatraceCredentials(dtCreds, data.Project, stdLogger)

	if err != nil {
		stdLogger.Error("Failed to fetch Dynatrace credentials: " + err.Error())
		return err
	}

	// Prepare the folder structure for monaco (create base + shkeptncontext temp folder, copy files, get monaco.zip, extract and copy to temp)
	err = common.PrepareFiles(keptnEvent, keptnEvent.Context, stdLogger)
	if err != nil {
		stdLogger.Error(fmt.Sprintf("Error preparing monaco files: %s", err.Error()))
		return err
	}

	// generate projects string for monaco
	monacoProjects := common.GenerateMonacoProjectStringFromMonacoConfig(monacoConfigFile, keptnEvent)

	// test and apply monaco configuration
	err = callMonaco(dtCredentials, keptnEvent.Context, data, monacoProjects, stdLogger)

	// Clean up: remove temp folder for Context
	err = common.DeleteTempFolderForKeptnContext(keptnEvent.Context)
	stdLogger.Info(fmt.Sprintf("Delete temp folder for %s", keptnEvent.Context))

	return nil
}

//
// Handles DeploymentFinishedEventType = "sh.keptn.events.deployment-finished"
// TODO: add in your handler code
//
func HandleDeploymentFinishedEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.DeploymentFinishedEventData) error {
	//log.Printf("Handling Deployment Finished Event: %s", incomingEvent.Context.GetID())
	return nil
}

//
// Handles TestsFinishedEventType = "sh.keptn.events.tests-finished"
// TODO: add in your handler code
//
func HandleTestsFinishedEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.TestsFinishedEventData) error {
	//log.Printf("Handling Tests Finished Event: %s", incomingEvent.Context.GetID())

	return nil
}

//
// Handles EvaluationDoneEventType = "sh.keptn.events.evaluation-done"
// TODO: add in your handler code
//
func HandleStartEvaluationEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.StartEvaluationEventData) error {
	//log.Printf("Handling Start Evaluation Event: %s", incomingEvent.Context.GetID())

	return nil
}

//
// Handles DeploymentFinishedEventType = "sh.keptn.events.deployment-finished"
// TODO: add in your handler code
//
func HandleEvaluationDoneEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.EvaluationDoneEventData) error {
	//log.Printf("Handling Evaluation Done Event: %s", incomingEvent.Context.GetID())

	return nil
}

//
// Handles InternalGetSLIEventType = "sh.keptn.internal.event.get-sli"
// TODO: add in your handler code
//
func HandleInternalGetSLIEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.InternalGetSLIEventData) error {

	return nil
}

//
// Handles ProblemOpenEventType = "sh.keptn.event.problem.open"
// Handles ProblemEventType = "sh.keptn.events.problem"
// TODO: add in your handler code
//
func HandleProblemEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.ProblemEventData) error {
	//log.Printf("Handling Problem Event: %s", incomingEvent.Context.GetID())
	// Deprecated since Keptn 0.7.0 - use the HandleActionTriggeredEvent instead
	return nil
}

//
// Handles ActionTriggeredEventType = "sh.keptn.event.action.triggered"
// TODO: add in your handler code
//
func HandleActionTriggeredEvent(myKeptn *keptn.Keptn, incomingEvent cloudevents.Event, data *keptn.ActionTriggeredEventData) error {
	//log.Printf("Handling Action Triggered Event: %s", incomingEvent.Context.GetID())
	return nil
}

func getDynatraceCredentials(secretName string, project string, logger *keptnlog.Logger) (*common.DTCredentials, error) {

	secretNames := []string{secretName, fmt.Sprintf("dynatrace-credentials-%s", project), "dynatrace-credentials", "dynatrace"}

	for _, secret := range secretNames {
		if secret == "" {
			continue
		}

		dtCredentials, err := common.GetDTCredentials(secret)

		if err != nil {
			logger.Error(fmt.Sprintf("Error retrieving secret '%s': %v", secret, err))
		}

		if err == nil && dtCredentials != nil {
			// lets validate if the tenant URL is
			logger.Info(fmt.Sprintf("Secret '%s' with credentials found, returning (%s) ...", secret, dtCredentials.Tenant))
			return dtCredentials, nil
		}
	}

	return nil, errors.New("Could not find any Dynatrace specific secrets with the following names: " + strings.Join(secretNames, ","))
}

func callMonaco(dtCredentials *common.DTCredentials, keptnContext string, keptnEvent *keptnlib.ConfigurationChangeEventData, projects string, logger *keptnlog.Logger) error {
	// Dry Run to test configuration structure
	err := common.ExecuteMonaco(dtCredentials, keptnContext, keptnEvent, projects, true, true)
	if err != nil {
		return err
	}

	// Apply configuration
	err = common.ExecuteMonaco(dtCredentials, keptnContext, keptnEvent, projects, true, false)

	return err
}
