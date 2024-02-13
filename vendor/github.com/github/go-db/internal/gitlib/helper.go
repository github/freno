// Package gitlib implements a helper library meant to simplify the process of creating a branch, making changes and opening a pull request.
package gitlib

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/github"
)

// Helper interface defines the methods we want to use to interact with repos via the GitHub API.
type Helper interface {
	NewBranchFromMaster(repo, name string) (Branchable, error)
	GetExistingBranch(repo, name string) (Branchable, error)
}

// APIHelper wraps the github.Client and adds some convienience methods
// This is the concrete implementation of the Helper interface.
type APIHelper struct {
	*github.Client
}

// RepoOwner is the username for all internal GH repos.
const RepoOwner = "github"

// NewHelper constructs a concrete APIHelper object with the Helper interface type.
func NewHelper(c *github.Client) Helper {
	return &APIHelper{Client: c}
}

// HubotHasAdmin checks permissions on the given repo.
func (h *APIHelper) HubotHasAdmin(repo string) bool {
	permissionLevel, apiResp, err := h.Client.Repositories.GetPermissionLevel(
		context.Background(), RepoOwner, repo, "hubot")
	if err != nil ||
		apiResp.Response.StatusCode != http.StatusOK ||
		*permissionLevel.Permission != "admin" {
		return false
	}
	return true
}

// NewBranchFromMaster creates a branch from master with the specified name.
func (h *APIHelper) NewBranchFromMaster(repo, name string) (Branchable, error) {
	master, resp, err := h.Client.Repositories.GetBranch(context.Background(),
		RepoOwner, repo, "master")
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}

	return &Branch{
		Client: h.Client,
		Repo:   repo,
		Name:   name,
		Base:   master,
	}, nil
}

// GetExistingBranch gets a branch by the name provided; it must exist already.
func (h *APIHelper) GetExistingBranch(repo, name string) (Branchable, error) {
	master, resp, err := h.Client.Repositories.GetBranch(context.Background(),
		RepoOwner, repo, name)
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}

	refs, resp, err := h.Client.Git.GetRefs(context.Background(),
		RepoOwner, repo, fmt.Sprintf("heads/%s", name))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil, err
	}

	return &Branch{
		Client: h.Client,
		Repo:   repo,
		Name:   name,
		Base:   master,
		Refs:   refs,
	}, nil
}
