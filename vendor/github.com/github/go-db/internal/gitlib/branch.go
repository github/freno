package gitlib

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
)

// Branchable interface defines how we can interact with GitHub branches.
type Branchable interface {
	AddFiles(files []FileContents) error
	CommitChanges(message string) error
	CreatePR(title, body string) (*github.PullRequest, error)
	GetFileBlob(path string) ([]byte, error)
	GetFile(path string) (*github.TreeEntry, error)
	GetName() string
	GetRefs() []*github.Reference
	GetChanges() []github.TreeEntry
}

// Branch represents a branch from master to which we can add changes.
type Branch struct {
	Client  *github.Client
	Repo    string
	Name    string
	Base    *github.Branch
	Tree    *github.Tree
	Changes []github.TreeEntry
	Refs    []*github.Reference
}

// FileContents is the properties we need to add a file to the repo.
type FileContents struct {
	Path     string
	Mode     string
	Contents string
}

var (
	// ErrInvalidGetPath is returned by GetFile when a malformed path is provided.
	ErrInvalidGetPath = errors.New("no path provided in GetFile")
	// ErrFileNotFound is returned by GetFile when a file couldn't be found at the provided path.
	ErrFileNotFound = errors.New("file not found in repo")
)

// AddFiles uploads and creates references for a list of files.
func (b *Branch) AddFiles(files []FileContents) error {
	encoding := "utf-8"
	for _, f := range files {
		f := f // Workaround: this copies the range loop variable to be able to take its address.
		ref := github.Blob{
			Content:  &f.Contents,
			Encoding: &encoding,
		}

		blob, resp, err := b.Client.Git.CreateBlob(context.Background(),
			RepoOwner, b.Repo, &ref)
		if err != nil {
			return err
		}

		if resp.StatusCode != http.StatusCreated {
			return fmt.Errorf("failed to create file: %d", resp.StatusCode)
		}

		sha := blob.GetSHA()
		path := f.Path // copying b/c we need to take a reference
		mode := f.Mode
		entry := github.TreeEntry{
			SHA:  &sha,
			Path: &path,
			Mode: &mode,
		}

		b.Changes = append(b.Changes, entry)
	}

	return nil
}

// CommitChanges updates the working tree for a branch and does a commit.
func (b *Branch) CommitChanges(message string) error {
	// start with the master tree then apply changes
	tree := b.Base.GetCommit().GetCommit().GetTree()
	newTree, resp, err := b.Client.Git.CreateTree(context.Background(),
		RepoOwner, b.Repo, tree.GetSHA(), b.Changes)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create new tree to apply commits: %d", resp.StatusCode)
	}

	b.Tree = newTree

	masterSHA := b.Base.GetCommit().GetSHA()
	parentCommits := []github.Commit{
		{
			SHA: &masterSHA,
		},
	}

	commit := &github.Commit{
		Message: &message,
		Tree:    b.Tree,
		Parents: parentCommits,
	}

	newCommit, resp, err := b.Client.Git.CreateCommit(context.Background(),
		RepoOwner, b.Repo, commit)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed to create commit: %d", resp.StatusCode)
	}

	ref := "refs/heads/" + b.Name
	objectType := "commit"
	commitSha := newCommit.GetSHA()
	branchRef := &github.Reference{
		Ref: &ref,
		Object: &github.GitObject{
			Type: &objectType,
			SHA:  &commitSha,
		},
	}

	if len(b.Refs) == 0 {
		// Attach the commit to a branch if this is the first commit
		newRef, resp, err := b.Client.Git.CreateRef(context.Background(),
			RepoOwner, b.Repo, branchRef)
		if err != nil || resp.StatusCode != http.StatusCreated {
			return err
		}
		b.Refs = append(b.Refs, newRef)
	} else {
		// Otherwise if we have existing commits, just update HEAD
		newRef, resp, err := b.Client.Git.UpdateRef(context.Background(),
			RepoOwner, b.Repo, branchRef, false)
		if err != nil || resp.StatusCode != http.StatusCreated {
			return err
		}
		b.Refs = append(b.Refs, newRef)
	}

	return nil
}

// CreatePR takes a branch and creates a Pull Request.
func (b *Branch) CreatePR(title, body string) (*github.PullRequest, error) {
	request := github.NewPullRequest{
		Title: &title,
		Head:  &b.Name,
		Base:  b.Base.Name,
		Body:  &body,
	}

	pr, resp, err := b.Client.PullRequests.Create(context.Background(),
		RepoOwner, b.Repo, &request)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create Pull Request: %d", resp.StatusCode)
	}

	return pr, nil
}

// GetFile iteratively calls GetTree on a file path until the file TreeEntry object
// is reached.  This "gets" a file without having to load the entire tree for the repo.
func (b *Branch) GetFile(path string) (*github.TreeEntry, error) {
	var tree *github.Tree
	var err error

	if path[0] == '/' {
		path = path[1:]
	}
	parts := strings.Split(path, "/")

	if len(parts) == 0 || path == "" {
		return nil, ErrInvalidGetPath
	}

	parent := b.Base.GetCommit().GetCommit().GetTree().GetSHA()
	parentTree, _, err := b.Client.Git.GetTree(
		context.Background(), RepoOwner, b.Repo, parent, false)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(parts)-1; i++ {
		tree, err = b.getSubdirAsTree(parentTree, parts[i])
		if err != nil {
			return nil, err
		}
		if tree == nil {
			return nil, errors.New("unexpected nil tree in GetFile")
		}
		parentTree = tree
	}

	// here parent should point to the directory containing the file we want
	for _, entry := range parentTree.Entries {
		if entry.GetPath() == parts[len(parts)-1] {
			return &entry, nil
		}
	}
	return nil, ErrFileNotFound
}

func (b *Branch) getSubdirAsTree(parentTree *github.Tree, dir string) (*github.Tree, error) {
	var target *github.TreeEntry
	for _, entry := range parentTree.Entries {
		if entry.GetPath() == dir {
			// copy because the iterator var changes
			tmp := entry
			target = &tmp
		}
	}

	if target != nil {
		tree, _, err := b.Client.Git.GetTree(
			context.Background(), RepoOwner, b.Repo, target.GetSHA(), false)

		if err != nil {
			return nil, err
		}

		return tree, nil
	}

	return nil, ErrFileNotFound
}

// GetFileBlob calls the GitHub API to retrieve the file contents & decodes them.
func (b *Branch) GetFileBlob(path string) ([]byte, error) {
	file, err := b.GetFile(path)
	if err != nil {
		return nil, err
	}

	blob, _, err := b.Client.Git.GetBlob(context.Background(), RepoOwner, b.Repo, file.GetSHA())
	if err != nil {
		return nil, err
	}

	data, err := base64.StdEncoding.DecodeString(blob.GetContent())
	if err != nil {
		return nil, err
	}

	return data, nil
}

// GetName returns the branch name.
func (b *Branch) GetName() string { return b.Name }

// GetRefs returns the list of Ref objects associated with the branch.
func (b *Branch) GetRefs() []*github.Reference {
	return b.Refs
}

// GetChanges returns an array of github.TreeEntry objects representing files
// That have been added with AddFiles().
func (b *Branch) GetChanges() []github.TreeEntry {
	return b.Changes
}
