package gitlib

import "github.com/google/go-github/github"

// MockBranch is used for testing.
type MockBranch struct {
	Name  string
	Files []FileContents
}

// AddFiles adds files to a branch to be committed.
func (b *MockBranch) AddFiles(files []FileContents) error {
	b.Files = append(b.Files, files...)
	return nil
}

// CommitChanges saves the changes to the branch.
func (b *MockBranch) CommitChanges(message string) error {
	return nil
}

// CreatePR creates the pull request from changes on the branch.
func (b *MockBranch) CreatePR(title, body string) (*github.PullRequest, error) {
	url := "http://github.com/github/db-scripts/pull/1"
	return &github.PullRequest{
		HTMLURL: &url,
	}, nil
}

// GetFileBlob returns the file contents for the path given.
func (b *MockBranch) GetFileBlob(path string) ([]byte, error) {
	for _, file := range b.Files {
		if path == file.Path {
			return []byte(file.Contents), nil
		}
	}
	return []byte{}, nil
}

// GetFile returns the object properties for the given path.
func (b *MockBranch) GetFile(path string) (*github.TreeEntry, error) {
	return nil, nil
}

// GetName returns the name of the branch.
func (b *MockBranch) GetName() string { return b.Name }

// GetRefs returns the list of Ref objects associated with the branch.
func (b *MockBranch) GetRefs() []*github.Reference {
	return nil
}

// GetChanges returns an array of github.TreeEntry objects.
func (b *MockBranch) GetChanges() []github.TreeEntry {
	return []github.TreeEntry{}
}
