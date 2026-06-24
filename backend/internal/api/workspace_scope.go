package api

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	workspaceProjectHeader     = "X-AIT-Project-ID"
	workspaceEnvironmentHeader = "X-AIT-Environment"
)

type workspaceScopeConstraint struct {
	ProjectID   string
	Environment string
}

func workspaceScopeFromRequest(r *http.Request) (workspaceScopeConstraint, bool) {
	projectID := strings.TrimSpace(r.Header.Get(workspaceProjectHeader))
	environment := strings.TrimSpace(r.Header.Get(workspaceEnvironmentHeader))
	if projectID == "" {
		projectID = strings.TrimSpace(r.URL.Query().Get("projectId"))
	}
	if environment == "" {
		environment = strings.TrimSpace(r.URL.Query().Get("environment"))
	}
	if projectID == "" && environment == "" {
		return workspaceScopeConstraint{}, false
	}
	if projectID == "" {
		projectID = defaultScenarioProjectID
	}
	if environment == "" {
		environment = defaultScenarioEnvironment
	}
	return workspaceScopeConstraint{
		ProjectID:   projectID,
		Environment: environment,
	}, true
}

func applyWorkspaceScopeToFilter(r *http.Request, projectID *string, environment *string) error {
	scope, ok := workspaceScopeFromRequest(r)
	if !ok {
		return nil
	}

	requestedProjectID := strings.TrimSpace(*projectID)
	requestedEnvironment := strings.TrimSpace(*environment)
	if requestedProjectID != "" && requestedProjectID != scope.ProjectID {
		return workspaceScopeError(requestedProjectID, requestedEnvironment, scope)
	}
	if requestedEnvironment != "" && requestedEnvironment != scope.Environment {
		return workspaceScopeError(requestedProjectID, requestedEnvironment, scope)
	}

	*projectID = scope.ProjectID
	*environment = scope.Environment
	return nil
}

func applyWorkspaceScopeToRecord(r *http.Request, projectID *string, environment *string) error {
	scope, ok := workspaceScopeFromRequest(r)
	if !ok {
		return nil
	}

	requestedProjectID := strings.TrimSpace(*projectID)
	requestedEnvironment := strings.TrimSpace(*environment)
	if requestedProjectID != "" && requestedProjectID != scope.ProjectID {
		return workspaceScopeError(requestedProjectID, requestedEnvironment, scope)
	}
	if requestedEnvironment != "" && requestedEnvironment != scope.Environment {
		return workspaceScopeError(requestedProjectID, requestedEnvironment, scope)
	}

	*projectID = scope.ProjectID
	*environment = scope.Environment
	return nil
}

func requireWorkspaceScopeForRecord(r *http.Request, projectID string, environment string) error {
	scope, ok := workspaceScopeFromRequest(r)
	if !ok {
		return nil
	}

	recordProjectID := strings.TrimSpace(projectID)
	recordEnvironment := strings.TrimSpace(environment)
	if recordProjectID == "" {
		recordProjectID = defaultScenarioProjectID
	}
	if recordEnvironment == "" {
		recordEnvironment = defaultScenarioEnvironment
	}
	if recordProjectID != scope.ProjectID || recordEnvironment != scope.Environment {
		return workspaceScopeError(recordProjectID, recordEnvironment, scope)
	}
	return nil
}

func workspaceScopeError(projectID string, environment string, scope workspaceScopeConstraint) error {
	projectID = strings.TrimSpace(projectID)
	environment = strings.TrimSpace(environment)
	if projectID == "" {
		projectID = defaultScenarioProjectID
	}
	if environment == "" {
		environment = defaultScenarioEnvironment
	}
	return fmt.Errorf("workspace scope %s/%s cannot access %s/%s", scope.ProjectID, scope.Environment, projectID, environment)
}
