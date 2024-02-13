package gitlib

// MockHelper is used for testing.
type MockHelper struct {
	Branch *MockBranch
}

// NewBranchFromMaster mocks creating a new branch.
func (h *MockHelper) NewBranchFromMaster(repo, name string) (Branchable, error) {
	return h.Branch, nil
}

// GetExistingBranch mocks getting a branch that already exists.
func (h *MockHelper) GetExistingBranch(repo, name string) (Branchable, error) {
	return h.Branch, nil
}
