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

	record := normalizedWorkspaceScope(projectID, environment)
	recordProjectID := record.ProjectID
	recordEnvironment := record.Environment
	if recordProjectID != scope.ProjectID || recordEnvironment != scope.Environment {
		return workspaceScopeError(recordProjectID, recordEnvironment, scope)
	}
	return nil
}

func normalizedWorkspaceScope(projectID string, environment string) workspaceScopeConstraint {
	projectID = strings.TrimSpace(projectID)
	environment = strings.TrimSpace(environment)
	if projectID == "" {
		projectID = defaultScenarioProjectID
	}
	if environment == "" {
		environment = defaultScenarioEnvironment
	}
	return workspaceScopeConstraint{
		ProjectID:   projectID,
		Environment: environment,
	}
}

func workspaceScopesMatch(leftProjectID string, leftEnvironment string, rightProjectID string, rightEnvironment string) bool {
	left := normalizedWorkspaceScope(leftProjectID, leftEnvironment)
	right := normalizedWorkspaceScope(rightProjectID, rightEnvironment)
	return left.ProjectID == right.ProjectID && left.Environment == right.Environment
}

func workspaceScopeError(projectID string, environment string, scope workspaceScopeConstraint) error {
	record := normalizedWorkspaceScope(projectID, environment)
	return fmt.Errorf("workspace scope %s/%s cannot access %s/%s", scope.ProjectID, scope.Environment, record.ProjectID, record.Environment)
}
